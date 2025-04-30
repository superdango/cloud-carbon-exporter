package gcp

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"reflect"
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
	machinetypes "github.com/superdango/cloud-carbon-exporter/internal/gcp/data/machine_types"
	"github.com/superdango/cloud-carbon-exporter/model/carbon"
	"github.com/superdango/cloud-carbon-exporter/model/primitives"
	"golang.org/x/sync/errgroup"

	"google.golang.org/api/iterator"
	"google.golang.org/api/monitoring/v1"
)

type Asset string

type AssetsDiscoveryMap map[Asset][]string

type SubExplorer interface {
	collectMetrics(ctx context.Context, metrics chan *cloudcarbonexporter.Metric) error
	init(ctx context.Context, explorer *Explorer) error
}

type Explorer struct {
	monitoringClient   *monitoring.Service
	ProjectID          string
	cache              *cache.Memory
	gcpZones           Zones
	carbonIntensityMap carbon.IntensityMap

	machineTypes machinetypes.MachineTypes

	subExplorers map[Asset]SubExplorer

	apiCallsCounter *atomic.Int64
}

func NewExplorer() *Explorer {
	return &Explorer{
		apiCallsCounter:    new(atomic.Int64),
		carbonIntensityMap: carbon.NewGCPCarbonIntensityMap(),
		machineTypes:       machinetypes.MustLoad(),
		subExplorers: map[Asset]SubExplorer{
			"compute.googleapis.com/Instance":   new(InstancesExplorer),
			"compute.googleapis.com/Disk":       new(DisksExplorer),
			"compute.googleapis.com/RegionDisk": new(RegionDisksExplorer),
			"storage.googleapis.com/Bucket":     new(BucketsExplorer),
			"sqladmin.googleapis.com/Instance":  new(CloudSQLExplorer),
		},
	}
}

func (explorer *Explorer) SupportedServices() []string {
	services := make([]string, 0)
	for asset := range explorer.subExplorers {
		services = append(services, string(asset))
	}
	return services
}

func (explorer *Explorer) Init(ctx context.Context) (err error) {
	if explorer.ProjectID == "" {
		return fmt.Errorf("project id is not set")
	}

	explorer.cache = cache.NewMemory(ctx, 5*time.Minute)
	errg := new(errgroup.Group)
	errg.Go(func() error {
		assets, err := asset.NewClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create asset inventory client: %w", err)
		}

		return explorer.cache.SetDynamicIfNotExists(ctx, "discovery_map", explorer.discoveryMapCacheValue(assets))
	})

	errg.Go(func() error {
		err := explorer.loadZones(ctx)
		if err != nil {
			return fmt.Errorf("failed to load zones: %w", err)
		}
		return nil
	})

	errg.Go(func() error {
		explorer.monitoringClient, err = monitoring.NewService(ctx)
		if err != nil {
			return fmt.Errorf("failed to initialize gcp monitoring client: %w", err)
		}
		slog.Debug("monitoring client initialized")
		return nil
	})

	for service, subExplorer := range explorer.subExplorers {
		errg.Go(func() error {
			err := subExplorer.init(ctx, explorer)
			if err != nil {
				return fmt.Errorf("failed to initialize subexplorer (%s): %w", service, err)
			}
			slog.Debug("service initialized", "service", service)
			return nil
		})
	}

	return errg.Wait()
}

func (explorer *Explorer) loadZones(ctx context.Context) error {
	slog.Info("loading zones and regions infos")
	zonesClient, err := compute.NewZonesRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize zone rest client: %w", err)
	}
	it := zonesClient.List(ctx, &computepb.ListZonesRequest{Project: explorer.ProjectID})
	for {
		zone, err := it.Next()
		if err == iterator.Done {
			explorer.apiCallsCounter.Add(1)
			break
		}
		if err != nil {
			return fmt.Errorf("failed to get zone: %w", err)
		}

		region := lastURLPathFragment(*zone.Region)
		explorer.gcpZones = append(explorer.gcpZones, Zone{
			Name:   *zone.Name,
			Region: region,
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

func (explorer *Explorer) GetCachedDiscoveryMap(ctx context.Context) (AssetsDiscoveryMap, error) {
	v, err := explorer.cache.Get(ctx, "discovery_map")
	if err != nil {
		return nil, err
	}

	assets, ok := v.(AssetsDiscoveryMap)
	if !ok {
		return nil, fmt.Errorf("wrong cache entry type: %s, expected AssetsDiscoveryMap", reflect.TypeOf(v))
	}

	return assets, nil
}

func (explorer *Explorer) CollectMetrics(ctx context.Context, metrics chan *cloudcarbonexporter.Metric, errs chan error) {
	explorer.apiCallsCounter.Store(0) // reset api calls counter
	energyMetrics := make(chan *cloudcarbonexporter.Metric)

	wg := new(sync.WaitGroup)
	wg.Add(1)

	go func() {
		defer wg.Done()
		for energyMetric := range energyMetrics {
			energyMetric.AddLabel("cloud_provider", "gcp")
			energyMetric.Value *= primitives.GoodPUE
			metrics <- energyMetric
			metrics <- explorer.carbonIntensityMap.ComputeCO2eq(energyMetric)
		}
	}()

	go func() {
		defer close(energyMetrics)
		explorer.collectMetrics(ctx, energyMetrics, errs)
	}()

	wg.Wait()

	metrics <- &cloudcarbonexporter.Metric{
		Name: "api_calls",
		Labels: map[string]string{
			"provider": "gcp",
		},
		Value: float64(explorer.apiCallsCounter.Load()),
	}
}

func (explorer *Explorer) collectMetrics(ctx context.Context, energyMetrics chan *cloudcarbonexporter.Metric, errs chan error) {
	discoveryMap, err := explorer.GetCachedDiscoveryMap(ctx)
	if err != nil {
		errs <- fmt.Errorf("failed to get cached discovery map: %w", err)
		return
	}

	wg := new(sync.WaitGroup)
	for _, assetName := range discoveryMap["types"] {
		subExplorer, found := explorer.subExplorers[Asset(assetName)]
		if !found {
			slog.Debug("asset is not supported", "asset", assetName)
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- subExplorer.collectMetrics(ctx, energyMetrics)
		}()

	}

	wg.Wait()
}

func (explorer *Explorer) discoveryMapCacheValue(client *asset.Client) cache.DynamicValueFunc {
	return cache.DynamicValueFunc(func(ctx context.Context) (any, error) {
		start := time.Now()
		req := &assetpb.ListAssetsRequest{
			Parent:      fmt.Sprintf("projects/%s", explorer.ProjectID),
			ContentType: assetpb.ContentType_RESOURCE,
		}

		assetTypes := make([]string, 0)
		activeZones := make([]string, 0)
		activeRegions := make([]string, 0)

		it := client.ListAssets(ctx, req)
		for {
			asset, err := it.Next()
			if err == iterator.Done {
				explorer.apiCallsCounter.Add(1)
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

		slog.Debug("assets listed", "projectID", explorer.ProjectID, "duration_ms", time.Since(start))

		return AssetsDiscoveryMap{
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

func (explorer *Explorer) Close() error {
	return nil
}

func (explorer *Explorer) IsReady() bool {
	return true
}
