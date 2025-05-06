package gcp

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
	"github.com/superdango/cloud-carbon-exporter/internal/must"
	"github.com/superdango/cloud-carbon-exporter/model/cloud"
	"github.com/superdango/cloud-carbon-exporter/model/primitives"
	"golang.org/x/sync/errgroup"
	"google.golang.org/api/iterator"
)

type InstancesExplorer struct {
	*Explorer
	client *compute.InstancesClient
	mu     *sync.Mutex
}

func (instanceExplorer *InstancesExplorer) init(ctx context.Context, explorer *Explorer) (err error) {
	instanceExplorer.Explorer = explorer
	instanceExplorer.mu = new(sync.Mutex)

	instanceExplorer.client, err = compute.NewInstancesRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create compute instances rest client: %w", err)
	}

	explorer.cache.SetDynamicIfNotExists(ctx, "instances_average_cpu", func(ctx context.Context) (any, error) {
		return instanceExplorer.ListInstanceCPUAverage(cloudcarbonexporter.WrapCtx(ctx))
	}, 5*time.Minute)

	return nil
}

func (instanceExplorer *InstancesExplorer) collectImpacts(ctx cloudcarbonexporter.Context, impacts chan *cloudcarbonexporter.Impact) error {
	discoveryMap, err := instanceExplorer.GetCachedDiscoveryMap(ctx)
	if err != nil {
		return fmt.Errorf("failed to get cached discovery map: %w", err)
	}

	errg := new(errgroup.Group)

	for _, zone := range discoveryMap["zones"] {
		errg.Go(func() error {
			return instanceExplorer.collectZoneImpacts(ctx, zone, impacts)
		})
	}
	return errg.Wait()
}

func (instanceExplorer *InstancesExplorer) collectZoneImpacts(ctx cloudcarbonexporter.Context, zone string, impacts chan *cloudcarbonexporter.Impact) error {
	instancesIter := instanceExplorer.client.List(ctx, &computepb.ListInstancesRequest{
		Project: instanceExplorer.ProjectID,
		Zone:    zone,
	})

	for {
		instance, err := instancesIter.Next()
		if err == iterator.Done {
			ctx.IncrCalls()
			break
		}
		if err != nil {
			return fmt.Errorf("failed to iterate on zone: %s next instance: %w", zone, err)
		}

		instanceName := instance.GetName()
		cpuPlatform := instance.GetCpuPlatform()
		processor := primitives.LookupProcessorByName(cpuPlatform)
		machineType := instanceExplorer.machineTypes.Get(lastURLPathFragment(instance.GetMachineType()))
		if machineType.Name == "unknown" {
			slog.Warn("unknown machine type", "machine_type", lastURLPathFragment(instance.GetMachineType()))
			continue
		}

		cpuUsage, err := instanceExplorer.GetInstanceAverageCPULoad(ctx, instanceName)
		if err != nil {
			return err
		}

		energy := processor.EstimateCPUEnergy(machineType.VCPU, cpuUsage)
		energy += primitives.EstimateMemoryEnergy(machineType.Memory)
		for _, disk := range instance.Disks {
			// Physical disks (SCRATCH) are directly attached to the instance
			if *disk.Type == "SCRATCH" {
				energy += primitives.EstimateLocalSSDEnergy(1)
			}
		}

		impacts <- &cloudcarbonexporter.Impact{
			Energy: cloudcarbonexporter.Energy(energy),
			Labels: cloudcarbonexporter.MergeLabels(
				map[string]string{
					"kind":          "compute/Instance",
					"instance_name": instanceName,
					"zone":          lastURLPathFragment(instance.GetZone()),
					"region":        instanceExplorer.gcpZones.GetRegion(lastURLPathFragment(instance.GetZone())),
					"location":      instanceExplorer.gcpZones.GetRegion(lastURLPathFragment(instance.GetZone())),
				},
				instance.Labels,
			),
		}
	}

	return nil
}

func (instanceExplorer *InstancesExplorer) GetInstanceAverageCPULoad(ctx context.Context, instanceName string) (float64, error) {
	// locking mutex prevents monitoring requests sent in parallel
	instanceExplorer.mu.Lock()
	defer instanceExplorer.mu.Unlock()

	entry, err := instanceExplorer.cache.Get(ctx, "instances_average_cpu")
	if err != nil {
		return 0, fmt.Errorf("failed to get explorer instance average cpu cache: %w", err)
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
func (explorer *InstancesExplorer) ListInstanceCPUAverage(ctx cloudcarbonexporter.Context) (map[string]float64, error) {
	promqlExpression := `avg by (instance_name)(rate(compute_googleapis_com:instance_cpu_usage_time{monitored_resource="gce_instance"}[5m]))`
	period := 10 * time.Minute

	instanceList, err := explorer.query(ctx, promqlExpression, "instance_name", period)
	if err != nil {
		return nil, fmt.Errorf("failed to query for instance monitoring data: %w", err)
	}

	ctx.IncrCalls()

	return instanceList, nil
}

type DisksExplorer struct {
	*Explorer
	client *compute.DisksClient
}

func (disksExplorer *DisksExplorer) init(ctx context.Context, explorer *Explorer) (err error) {
	disksExplorer.Explorer = explorer
	disksExplorer.client, err = compute.NewDisksRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create disks rest client: %w", err)
	}

	return nil
}

func (disksExplorer *DisksExplorer) collectImpacts(ctx cloudcarbonexporter.Context, impacts chan *cloudcarbonexporter.Impact) error {
	discoveryMap, err := disksExplorer.GetCachedDiscoveryMap(ctx)
	if err != nil {
		return fmt.Errorf("failed to get cached discovery map: %w", err)
	}

	errg := new(errgroup.Group)

	for _, zone := range discoveryMap["zones"] {
		errg.Go(func() error {
			return disksExplorer.collectZoneImpacts(ctx, zone, impacts)
		})
	}
	return errg.Wait()
}

func (disksExplorer *DisksExplorer) collectZoneImpacts(ctx cloudcarbonexporter.Context, zone string, impacts chan *cloudcarbonexporter.Impact) error {
	disksIter := disksExplorer.client.List(ctx, &computepb.ListDisksRequest{
		Project: disksExplorer.ProjectID,
		Zone:    zone,
	})

	for {
		disk, err := disksIter.Next()
		if err == iterator.Done {
			ctx.IncrCalls()
			break
		}
		if err != nil {
			return fmt.Errorf("failed to iterate on next disk (zone: %s): %w", zone, err)
		}

		diskName := disk.GetName()

		energy := cloudcarbonexporter.Energy(0)
		switch lastURLPathFragment(disk.GetType()) {
		case "pd-standard":
			energy = cloud.EstimateHDDBlockStorageEnergy(float64(*disk.SizeGb))
		default:
			energy = cloud.EstimateSSDBlockStorageEnergy(float64(*disk.SizeGb))
		}
		replicas := 1
		if len(disk.ReplicaZones) > 0 {
			replicas += len(disk.ReplicaZones)
		}

		energy = energy * cloudcarbonexporter.Energy(replicas)

		impacts <- &cloudcarbonexporter.Impact{
			Energy: cloudcarbonexporter.Energy(energy),
			Labels: cloudcarbonexporter.MergeLabels(
				map[string]string{
					"kind":      "compute/Disk",
					"disk_name": diskName,
					"zone":      lastURLPathFragment(disk.GetZone()),
					"location":  disksExplorer.gcpZones.GetRegion(lastURLPathFragment(disk.GetZone())),
				},
				disk.Labels,
			),
		}
	}

	return nil
}

type RegionDisksExplorer struct {
	*Explorer
	client *compute.RegionDisksClient
}

func (regionDisksExplorer *RegionDisksExplorer) init(ctx context.Context, explorer *Explorer) (err error) {
	regionDisksExplorer.Explorer = explorer
	regionDisksExplorer.client, err = compute.NewRegionDisksRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create region disk rest client: %w", err)
	}
	return nil
}

func (regionDisksExplorer *RegionDisksExplorer) collectImpacts(ctx cloudcarbonexporter.Context, impacts chan *cloudcarbonexporter.Impact) (err error) {
	discoveryMap, err := regionDisksExplorer.GetCachedDiscoveryMap(ctx)
	if err != nil {
		return fmt.Errorf("failed to get cached discovery map: %w", err)
	}

	errg := new(errgroup.Group)

	for _, region := range discoveryMap["regions"] {
		errg.Go(func() error {
			return regionDisksExplorer.collectRegionImpacts(ctx, region, impacts)
		})
	}
	return errg.Wait()
}

func (regionDisksExplorer *RegionDisksExplorer) collectRegionImpacts(ctx cloudcarbonexporter.Context, region string, impacts chan *cloudcarbonexporter.Impact) error {
	if region == "global" {
		return nil
	}

	regionDisksIter := regionDisksExplorer.client.List(ctx, &computepb.ListRegionDisksRequest{
		Project: regionDisksExplorer.ProjectID,
		Region:  region,
	})

	for {
		disk, err := regionDisksIter.Next()
		if err == iterator.Done {
			ctx.IncrCalls()
			break
		}
		if err != nil {
			return fmt.Errorf("failed to iterate on next disk: %w", err)
		}

		diskName := disk.GetName()
		fmt.Println("got region disk", diskName, region)

		energy := cloudcarbonexporter.Energy(0)
		switch lastURLPathFragment(disk.GetType()) {
		case "pd-standard":
			energy = cloud.EstimateHDDBlockStorageEnergy(float64(*disk.SizeGb))
		default:
			energy = cloud.EstimateSSDBlockStorageEnergy(float64(*disk.SizeGb))
		}

		replicas := 2

		energy = energy * cloudcarbonexporter.Energy(replicas)

		impacts <- &cloudcarbonexporter.Impact{
			Energy: cloudcarbonexporter.Energy(energy),
			Labels: cloudcarbonexporter.MergeLabels(
				map[string]string{
					"kind":      "compute/RegionDisk",
					"disk_name": diskName,
					"location":  region,
				},
				disk.Labels,
			),
		}
	}

	return nil
}
