package gcp

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
	"github.com/superdango/cloud-carbon-exporter/internal/must"
	"github.com/superdango/cloud-carbon-exporter/model/cloud"
	"github.com/superdango/cloud-carbon-exporter/model/primitives"
	cloudsql "google.golang.org/api/sqladmin/v1"
)

type CloudSQLExplorer struct {
	*Explorer
	client *cloudsql.Service
	mu     *sync.Mutex
}

func (sqlExplorer *CloudSQLExplorer) init(ctx context.Context, explorer *Explorer) (err error) {
	sqlExplorer.Explorer = explorer
	sqlExplorer.mu = new(sync.Mutex)

	sqlExplorer.client, err = cloudsql.NewService(ctx)
	if err != nil {
		return fmt.Errorf("failed to create cloudsql service: %w", err)
	}

	sqlExplorer.cache.SetDynamicIfNotExists(ctx, "sql_instances_average_cpu", func(ctx context.Context) (any, error) {
		return sqlExplorer.ListSQLInstanceCPUAverage(cloudcarbonexporter.WrapCtx(ctx))
	}, 5*time.Minute)

	return nil
}

func (sqlExplorer *CloudSQLExplorer) collectImpacts(ctx cloudcarbonexporter.Context, impacts chan *cloudcarbonexporter.Impact) error {
	ctx.IncrCalls()
	return sqlExplorer.client.Instances.List(sqlExplorer.ProjectID).Context(ctx).Pages(ctx, func(instancesList *cloudsql.InstancesListResponse) error {
		for _, instance := range instancesList.Items {
			var energy cloudcarbonexporter.Energy

			machineTypeName := strings.TrimPrefix(instance.Settings.Tier, "db-")
			machineType := sqlExplorer.machineTypes.Get(machineTypeName)
			if machineType.Name == "unknown" {
				return fmt.Errorf("unknown sql machine type: %s", machineTypeName)
			}

			cpuUsage, err := sqlExplorer.GetCloudSQLInstanceAverageCPUUsage(ctx, instance.Name)
			if err != nil {
				return fmt.Errorf("failed to get cloudsql intance cpu usage: %w", err)
			}

			// CPU
			energy += primitives.LookupProcessorByName(machineType.CPUPlatform).EstimateCPUEnergy(machineType.VCPU, cpuUsage)
			cpuEmbodied := primitives.EstimateCPUEmbodiedEmissions(machineType.VCPU)

			// Memory
			energy += primitives.EstimateMemoryEnergy(machineType.Memory)
			memoryEmbodied := primitives.EstimateMemoryEmbodiedEmissions(machineType.Memory)

			// Disk
			diskEnergy := cloud.EstimateSSDBlockStorageEnergy(float64(instance.Settings.DataDiskSizeGb))
			diskEmbodied := primitives.EstimateEmbodiedSSDEmissions(float64(instance.Settings.DataDiskSizeGb))
			if instance.Settings.DataDiskType != "PD_SSD" {
				diskEnergy = cloud.EstimateHDDBlockStorageEnergy(float64(instance.Settings.DataDiskSizeGb))
				diskEmbodied = primitives.EstimateEmbodiedHDDEmissions(float64(instance.Settings.DataDiskSizeGb))
			}
			energy += diskEnergy

			impacts <- &cloudcarbonexporter.Impact{
				Energy:            cloudcarbonexporter.Energy(energy),
				EmbodiedEmissions: cloudcarbonexporter.CombineEmissionsOverTime(cpuEmbodied, memoryEmbodied, diskEmbodied),
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
func (sqlExplorer *CloudSQLExplorer) ListSQLInstanceCPUAverage(ctx cloudcarbonexporter.Context) (map[string]float64, error) {
	promqlExpression := `avg by (database_id)(avg_over_time(cloudsql_googleapis_com:database_cpu_utilization{monitored_resource="cloudsql_database"}[5m]))`
	period := 10 * time.Minute

	instanceList, err := sqlExplorer.query(ctx, promqlExpression, "database_id", period)
	if err != nil {
		return nil, fmt.Errorf("failed to query for cloudsql instance monitoring data: %w", err)
	}

	ctx.IncrCalls()

	return instanceList, nil
}
