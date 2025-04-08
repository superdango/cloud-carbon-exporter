package gcp

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	asset "cloud.google.com/go/asset/apiv1"
	"cloud.google.com/go/asset/apiv1/assetpb"
	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
	"github.com/superdango/cloud-carbon-exporter/internal/cache"
	"github.com/superdango/cloud-carbon-exporter/internal/must"
	"github.com/superdango/cloud-carbon-exporter/model/carbon"
	"github.com/superdango/cloud-carbon-exporter/model/energy/primitives"
	"golang.org/x/sync/errgroup"

	"google.golang.org/api/iterator"
	"google.golang.org/api/monitoring/v1"
)

type Option func(e *Explorer)

type Zone struct {
	Name      string
	Region    string
	Continent string
}

type Zones []Zone

func (zs Zones) GetRegion(location string) string {
	if location == "global" {
		return "global"
	}

	for _, zone := range zs {
		if zone.Region == location {
			return zone.Region
		}

		if zone.Name == location {
			return zone.Region
		}
	}

	return "global"
}

func (zs Zones) IsValidZone(location string) bool {
	for _, zone := range zs {
		if zone.Name == location {
			return true
		}
	}

	return false
}

type Explorer struct {
	assets             *asset.Client
	monitoringClient   *monitoring.Service
	projectID          string
	cache              *cache.Memory
	gcpZones           Zones
	carbonIntensityMap carbon.IntensityMap

	instances   *InstancesExplorer
	disks       *DisksExplorer
	regionDisks *RegionDisksExplorer
	buckets     *BucketsExplorer

	apiCalls *atomic.Int64
}

func WithProjectID(projectID string) Option {
	return func(e *Explorer) {
		e.projectID = projectID
	}
}

func NewExplorer(ctx context.Context, opts ...Option) (*Explorer, error) {
	var err error
	explorer := new(Explorer)
	explorer.cache = cache.NewMemory(ctx, 5*time.Minute)
	explorer.carbonIntensityMap = carbon.NewGCPCarbonIntensityMap()
	explorer.apiCalls = new(atomic.Int64)

	for _, c := range opts {
		c(explorer)
	}

	if explorer.projectID == "" {
		return nil, fmt.Errorf("project id is not set")
	}

	err = explorer.cache.Set(ctx, "assets_zones_regions", cache.DynamicValue(explorer.discoverServicesZonesRegions()))
	if err != nil {
		return nil, fmt.Errorf("failed to set assets zones and regions cache: %w", err)
	}

	errg, errgctx := errgroup.WithContext(ctx)
	errg.Go(func() error {
		explorer.assets, err = asset.NewClient(errgctx)
		if err != nil {
			return fmt.Errorf("failed to create asset inventory client: %w", err)
		}
		return nil
	})

	errg.Go(func() error {
		explorer.monitoringClient, err = monitoring.NewService(ctx)
		if err != nil {
			return fmt.Errorf("failed to initialize gcp monitoring client: %w", err)
		}
		return nil
	})

	errg.Go(func() error {
		explorer.instances, err = NewInstancesExplorer(ctx, explorer)
		if err != nil {
			return fmt.Errorf("failed to initialize instances explorer: %w", err)
		}
		return nil
	})

	errg.Go(func() error {
		explorer.disks, err = NewDisksExplorer(ctx, explorer)
		if err != nil {
			return fmt.Errorf("failed to initialize disks explorer: %w", err)
		}
		return nil
	})

	errg.Go(func() error {
		explorer.regionDisks, err = NewRegionDisksExplorer(ctx, explorer)
		if err != nil {
			return fmt.Errorf("failed to initialize region disks explorer: %w", err)
		}
		return nil
	})

	errg.Go(func() error {
		err = explorer.loadZones(ctx)
		if err != nil {
			return fmt.Errorf("failed to load zones: %w", err)
		}
		return nil
	})

	errg.Go(func() error {
		explorer.buckets, err = NewBucketsExplorer(ctx, explorer)
		if err != nil {
			return fmt.Errorf("failed to initialize buckets explorer: %w", err)
		}
		return nil
	})

	return explorer, errg.Wait()
}

func (explorer *Explorer) loadZones(ctx context.Context) error {
	slog.Info("loading zones and regions infos")
	zonesClient, err := compute.NewZonesRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize zone rest client: %w", err)
	}
	it := zonesClient.List(ctx, &computepb.ListZonesRequest{Project: explorer.projectID})
	for {
		zone, err := it.Next()
		if err == iterator.Done {
			explorer.apiCalls.Add(1)
			break
		}
		if err != nil {
			return fmt.Errorf("failed to get zone: %w", err)
		}

		explorer.gcpZones = append(explorer.gcpZones, Zone{
			Name:   *zone.Name,
			Region: lastURLPathFragment(*zone.Region),
		})
	}

	slog.Info("zones and regions successfully loaded", "zones", len(explorer.gcpZones))
	return nil
}

// lastURLPathFragment returns the last fragment of an url path
// and return an empty string if no fragments are found
func lastURLPathFragment(sourceURL string) string {
	fragments := fragmentURLPath(sourceURL)
	return fragments[len(fragments)-1]
}

// fragmentURLPath returns all path fragments found in source url
// http://my.example.com/foo/bar would return ["foo", "bar"]
// if no fragments are found, it returns [""]
func fragmentURLPath(source string) []string {
	u, err := url.Parse(source)
	if err != nil {
		slog.Warn("cannot parse source url", "err", err.Error())
		return []string{""}
	}

	path := strings.TrimPrefix(u.Path, "/")
	path = strings.TrimSuffix(path, "/")

	return strings.Split(path, "/")
}

func (explorer *Explorer) CollectMetrics(ctx context.Context, metrics chan *cloudcarbonexporter.Metric, errs chan error) {
	explorer.apiCalls.Store(0)
	energyMetrics := make(chan *cloudcarbonexporter.Metric)

	wg := new(sync.WaitGroup)
	wg.Add(1)

	go func() {
		defer wg.Done()
		for energyMetric := range energyMetrics {
			energyMetric.SetLabel("cloud_provider", "gcp")
			energyMetric.Value *= primitives.GoodPUE
			metrics <- energyMetric
			metrics <- explorer.carbonIntensityMap.ComputeCO2eq(energyMetric)
		}
	}()

	go func() {
		defer close(energyMetrics)
		explorer.collectMetrics(ctx, metrics, errs, energyMetrics)
	}()

	wg.Wait()
}

func (explorer *Explorer) collectMetrics(ctx context.Context, metrics chan *cloudcarbonexporter.Metric, errs chan error, energyMetrics chan *cloudcarbonexporter.Metric) {
	v, err := explorer.cache.Get(ctx, "assets_zones_regions")
	if err != nil {
		errs <- err
		return
	}

	assets, ok := v.(map[string][]string)
	must.Assert(ok, "wrong cache entry type")

	wg := new(sync.WaitGroup)
	for _, assetType := range assets["types"] {
		switch assetType {
		case "compute.googleapis.com/Instance":
			for _, zone := range assets["zones"] {
				async(wg, func() { errs <- explorer.instances.collectMetrics(ctx, zone, energyMetrics) })
			}
		case "compute.googleapis.com/Disk":
			for _, zone := range assets["zones"] {
				async(wg, func() { errs <- explorer.disks.collectMetrics(ctx, zone, energyMetrics) })
			}
		case "compute.googleapis.com/RegionDisk":
			for _, region := range assets["regions"] {
				async(wg, func() { errs <- explorer.regionDisks.collectMetrics(ctx, region, energyMetrics) })
			}
		case "storage.googleapis.com/Bucket":
			async(wg, func() { errs <- explorer.buckets.collectMetrics(ctx, energyMetrics) })
		default:
			slog.Debug("asset type is not supported", "asset", assetType)
		}
	}

	wg.Wait()

	metrics <- &cloudcarbonexporter.Metric{
		Name: "api_calls",
		Labels: map[string]string{
			"provider": "gcp",
		},
		Value: float64(explorer.apiCalls.Load()),
	}
}

func (explorer *Explorer) discoverServicesZonesRegions() cache.DynamicValue {
	return cache.DynamicValue(func(ctx context.Context) (any, error) {
		slog.Debug("listing assets", "projectID", explorer.projectID)
		req := &assetpb.ListAssetsRequest{
			Parent:      fmt.Sprintf("projects/%s", explorer.projectID),
			ContentType: assetpb.ContentType_RESOURCE,
		}

		assetTypes := make([]string, 0)
		activeZones := make([]string, 0)
		activeRegions := make([]string, 0)

		it := explorer.assets.ListAssets(ctx, req)
		for {
			asset, err := it.Next()
			if err == iterator.Done {
				explorer.apiCalls.Add(1)
				break
			}

			if err != nil {
				return nil, &cloudcarbonexporter.ExplorerErr{Err: fmt.Errorf("failed to list assets inventory resources: %w", err), Operation: "asset/apiv1:ListAssets"}
			}

			assetTypes = append(assetTypes, asset.AssetType)
			if explorer.gcpZones.IsValidZone(asset.Resource.Location) {
				activeZones = append(activeZones, asset.Resource.Location)
			}
			activeRegions = append(activeRegions, explorer.gcpZones.GetRegion(asset.Resource.Location))

		}

		assetTypes = distinct(assetTypes)
		activeZones = distinct(activeZones)
		activeRegions = distinct(activeRegions)

		return map[string][]string{
			"types":   assetTypes,
			"zones":   activeZones,
			"regions": activeRegions,
		}, nil
	})
}

// distinct remove all duplicates in string slice
func distinct(sl []string) []string {
	nsl := make([]string, 0)
	m := make(map[string]bool)
	for _, s := range sl {
		m[s] = false
	}
	for k := range m {
		nsl = append(nsl, k)
	}

	return nsl
}

func async(wg *sync.WaitGroup, fn func()) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		fn()
	}()
}

func (explorer *Explorer) Close() error {
	return explorer.assets.Close()
}

func (explorer *Explorer) IsReady() bool {
	return explorer.assets != nil
}
