package gcp

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	asset "cloud.google.com/go/asset/apiv1"
	"cloud.google.com/go/asset/apiv1/assetpb"
	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
	"github.com/superdango/cloud-carbon-exporter/internal/cache"
	"github.com/superdango/cloud-carbon-exporter/internal/must"
	"golang.org/x/sync/errgroup"

	"google.golang.org/api/iterator"
	"google.golang.org/api/monitoring/v1"
)

type Option func(e *Explorer)

type Explorer struct {
	assetClient      *asset.Client
	monitoringClient *monitoring.Service
	projectID        string
	cache            *cache.Memory
	refiners         map[string][]func(ctx context.Context, r *cloudcarbonexporter.Resource)
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

	explorer.cache = cache.NewMemory(5 * time.Minute)

	explorer.refiners = map[string][]func(ctx context.Context, r *cloudcarbonexporter.Resource){
		"compute.googleapis.com/Instance": {
			func(ctx context.Context, r *cloudcarbonexporter.Resource) {
				var err error
				r.Source["compute_instance_cpu_usage_percent"], err = explorer.GetInstanceCPUAverage(ctx, r.ID)
				if err != nil {
					slog.Warn("failed to run compute instance cpu average refiner", "id", r.ID, "err", err)
				}
				slog.Debug("compute cpu usage", "id", r.ID, "percent", r.Source["compute_instance_cpu_usage_percent"])
			},
		},
	}

	return explorer, nil
}

func (explorer *Explorer) Find(ctx context.Context, resources chan *cloudcarbonexporter.Resource) error {
	req := &assetpb.ListAssetsRequest{
		Parent:      fmt.Sprintf("projects/%s", explorer.projectID),
		ContentType: assetpb.ContentType_RESOURCE,
	}

	it := explorer.assetClient.ListAssets(ctx, req)
	for {
		response, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to iterate on the next asset: %w", err)
		}

		r := &cloudcarbonexporter.Resource{
			CloudProvider: "gcp",
			Kind:          response.AssetType,
			ID:            response.Resource.Data.GetFields()["name"].GetStringValue(),
			Region:        response.Resource.Location,
			Labels:        mapToStringMap(response.Resource.Data.AsMap()["labels"]),
			Source: map[string]any{
				"asset": response.Resource.Data.AsMap(),
			},
		}

		errg := new(errgroup.Group)
		if refiners, found := explorer.refiners[r.Kind]; found {
			for _, refiner := range refiners {
				errg.Go(func() error {
					refiner(ctx, r)
					return nil
				})
			}
		}
		_ = errg.Wait()

		resources <- r
	}

	return nil
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
