package gcp

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
	"github.com/superdango/cloud-carbon-exporter/internal/must"
	"github.com/superdango/cloud-carbon-exporter/model/energy/cloud"
	"github.com/superdango/cloud-carbon-exporter/model/energy/primitives"
	cloudsql "google.golang.org/api/sqladmin/v1"
)

type CloudSQLExplorer struct {
	*Explorer
	client *cloudsql.Service
	mu     *sync.Mutex
}

func NewCloudSQLExplorer(ctx context.Context, explorer *Explorer) (sqlExplorer *CloudSQLExplorer, err error) {
	sqlExplorer = &CloudSQLExplorer{
		Explorer: explorer,
		mu:       new(sync.Mutex),
	}

	sqlExplorer.client, err = cloudsql.NewService(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create cloudsql service: %w", err)
	}

	sqlExplorer.cache.SetDynamic(ctx, "sql_instances_average_cpu", func(ctx context.Context) (any, error) {
		return sqlExplorer.ListSQLInstanceCPUAverage(ctx)
	}, 5*time.Minute)

	return sqlExplorer, nil
}

func (sqlExplorer *CloudSQLExplorer) collectMetrics(ctx context.Context, metrics chan *cloudcarbonexporter.Metric) error {
	sqlExplorer.apiCallsCounter.Add(1)
	return sqlExplorer.client.Instances.List(sqlExplorer.projectID).Context(ctx).Pages(ctx, func(instancesList *cloudsql.InstancesListResponse) error {
		for _, instance := range instancesList.Items {
			watts := 0.0

			machineTypeName := strings.TrimPrefix(instance.Settings.Tier, "db-")
			machineType := sqlExplorer.machineTypes.Get(machineTypeName)
			if machineType.Name == "unknown" {
				return fmt.Errorf("unknown sql machine type: %s", machineTypeName)
			}

			cpuUsage, err := sqlExplorer.GetCloudSQLInstanceAverageCPUUsage(ctx, instance.Name)
			if err != nil {
				return fmt.Errorf("failed to get cloudsql intance cpu usage: %w", err)
			}

			watts += primitives.LookupProcessorByName(machineType.CPUPlatform).EstimateCPUWatts(machineType.VCPU, cpuUsage)

			diskWatts := cloud.EstimateSSDBlockStorageWatts(float64(instance.Settings.DataDiskSizeGb))
			if instance.Settings.DataDiskType != "PD_SSD" {
				diskWatts = cloud.EstimateHDDBlockStorageWatts(float64(instance.Settings.DataDiskSizeGb))
			}
			watts += diskWatts

			metrics <- &cloudcarbonexporter.Metric{
				Name: "estimated_watts",
				Labels: cloudcarbonexporter.MergeLabels(
					map[string]string{
						"kind":          "sql/Instance",
						"instance_name": instance.Name,
						"zone":          instance.GceZone,
						"region":        instance.Region,
						"location":      instance.Region,
					},
					instance.Settings.UserLabels,
				),
				Value: watts,
			}
		}
		return nil
	})
}

func (sqlExplorer *CloudSQLExplorer) GetCloudSQLInstanceAverageCPUUsage(ctx context.Context, instanceName string) (float64, error) {
	// locking mutex prevents monitoring requests sent in parallel
	sqlExplorer.mu.Lock()
	defer sqlExplorer.mu.Unlock()

	entry, err := sqlExplorer.cache.Get(ctx, "sql_instances_average_cpu")
	if err != nil {
		return 0, fmt.Errorf("failed to get explorer cloudsql instance average cpu cache: %w", err)
	}

	instancesAverageCPU, ok := entry.(map[string]float64)
	must.Assert(ok, "instancesAverageCPU is not a map[string]float64")

	instanceAverageCPU, found := instancesAverageCPU[instanceName]
	if !found {
		return 1.0, nil // minimum cpu average 1%
	}

	return instanceAverageCPU * 100, nil
}

// ListSQLInstanceCPUAverage returns the 10 minutes average cpu for all sql instances in the region
func (sqlExplorer *CloudSQLExplorer) ListSQLInstanceCPUAverage(ctx context.Context) (map[string]float64, error) {
	promqlExpression := `avg by (database_id)(avg_over_time(cloudsql_googleapis_com:database_cpu_utilization{monitored_resource="cloudsql_database"}[5m]))`
	period := 10 * time.Minute

	instanceList, err := sqlExplorer.query(ctx, promqlExpression, "database_id", period)
	if err != nil {
		return nil, fmt.Errorf("failed to query for cloudsql instance monitoring data: %w", err)
	}

	sqlExplorer.apiCallsCounter.Add(1)

	return instanceList, nil
}
