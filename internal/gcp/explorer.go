package gcp

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	asset "cloud.google.com/go/asset/apiv1"
	"cloud.google.com/go/asset/apiv1/assetpb"
	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
	"github.com/superdango/cloud-carbon-exporter/internal/cache"
	"github.com/superdango/cloud-carbon-exporter/internal/must"

	"google.golang.org/api/iterator"
	"google.golang.org/api/monitoring/v1"
)

type Option func(e *Explorer)

type Zone struct {
	Name   string
	Region string
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

func (zs Zones) IsZone(location string) bool {
	for _, zone := range zs {
		if zone.Name == location {
			return true
		}
	}

	return false
}

type Explorer struct {
	assetClient      *asset.Client
	monitoringClient *monitoring.Service
	projectID        string
	cache            *cache.Memory
	zones            Zones
}

func WithProjectID(projectID string) Option {
	return func(e *Explorer) {
		e.projectID = projectID
	}
}

func NewExplorer(ctx context.Context, opts ...Option) (*Explorer, error) {
	var err error
	explorer := new(Explorer)

	for _, c := range opts {
		c(explorer)
	}

	if explorer.projectID == "" {
		return nil, fmt.Errorf("project id is not set")
	}

	explorer.assetClient, err = asset.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create asset inventory client: %w", err)
	}

	explorer.monitoringClient, err = monitoring.NewService(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize gcp monitoring client: %w", err)
	}

	err = explorer.loadZones(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load zones: %w", err)
	}

	explorer.cache = cache.NewMemory(5 * time.Minute)

	return explorer, nil
}

func (explorer *Explorer) loadZones(ctx context.Context) error {
	zonesClient, err := compute.NewZonesRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize zone rest client: %w", err)
	}
	it := zonesClient.List(ctx, &computepb.ListZonesRequest{Project: explorer.projectID})
	for {
		zone, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to get zone: %w", err)
		}

		explorer.zones = append(explorer.zones, Zone{
			Name:   *zone.Name,
			Region: *zone.Region,
		})
	}
	return nil
}

func (explorer *Explorer) CollectMetrics(ctx context.Context, metrics chan *cloudcarbonexporter.Metric, errs chan error) {
	slog.Debug("listing assets", "projectID", explorer.projectID)
	req := &assetpb.ListAssetsRequest{
		Parent:      fmt.Sprintf("projects/%s", explorer.projectID),
		ContentType: assetpb.ContentType_RESOURCE,
	}

	it := explorer.assetClient.ListAssets(ctx, req)
	for {
		asset, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			errs <- &cloudcarbonexporter.ExplorerErr{Err: fmt.Errorf("failed to list assets inventory resources: %w", err), Operation: "asset/apiv1:ListAssets"}
			return
		}

		switch asset.AssetType {
		case "compute.googleapis.com/Instance":
			explorer.instanceEnergyMetric(ctx, asset, metrics, errs)
		}
	}
}

func (explorer *Explorer) Close() error {
	return explorer.assetClient.Close()
}

func (explorer *Explorer) IsReady() bool {
	return explorer.assetClient != nil
}

func (explorer *Explorer) GetInstanceCPUAverage(ctx context.Context, instanceName string) (float64, error) {
	key := "instances_average_cpu"
	entry, err := explorer.cache.GetOrSet(ctx, key, func(ctx context.Context) (any, error) {
		return explorer.ListInstanceCPUAverage(ctx)
	}, 5*time.Minute)
	if err != nil {
		return 1.0, fmt.Errorf("failed to list instance cpu average: %w", err)
	}

	instancesAverageCPU, ok := entry.(map[string]float64)
	must.Assert(ok, "instancesAverageCPU is not a map[string]float64")

	instanceAverageCPU, found := instancesAverageCPU[instanceName]
	if !found {
		return 1.0, nil // minimum cpu average 1%
	}

	return instanceAverageCPU * 100, nil
}

// ListInstanceCPUAverage returns the 10 minutes average cpu for all instances in the region
func (explorer *Explorer) ListInstanceCPUAverage(ctx context.Context) (map[string]float64, error) {
	promqlExpression := `avg by (instance_name)(rate(compute_googleapis_com:instance_cpu_usage_time{monitored_resource="gce_instance"}[5m]))`
	period := 10 * time.Minute

	instanceList, err := explorer.query(ctx, promqlExpression, "instance_name", period)
	if err != nil {
		return nil, fmt.Errorf("failed to query for instance monitoring data: %w", err)
	}

	return instanceList, nil
}

func mapToStringMap(m any) map[string]string {
	mapOfAny, ok := m.(map[string]any)
	if !ok {
		return make(map[string]string)
	}

	mapOfString := make(map[string]string)
	for k, v := range mapOfAny {
		mapOfString[k] = fmt.Sprintf("%s", v)
	}

	return mapOfString
}
