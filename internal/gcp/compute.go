package gcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
	machinetypes "github.com/superdango/cloud-carbon-exporter/internal/gcp/data/machine_types"
	"github.com/superdango/cloud-carbon-exporter/internal/must"
	"github.com/superdango/cloud-carbon-exporter/model/energy/cloud"
	"github.com/superdango/cloud-carbon-exporter/model/energy/primitives"
	"google.golang.org/api/iterator"
)

type MachineType struct {
	Name    string  `json:"name"`
	VCPU    float64 `json:"vcpu"`
	Memory  float64 `json:"memory"`
	GPU     float64 `json:"gpu"`
	GPUType string  `json:"gpu_type"`
}

type MachineTypes []MachineType

func (types MachineTypes) Get(name string) MachineType {
	// TODO: support custom instance type: e2-custom-2-14848
	//   isCustomeMachineType := strings.Contains(name, "-custom-")
	//   if isCustomMachineType {
	//   	...
	//   }

	for _, machineType := range types {
		if machineType.Name == name {
			return machineType
		}
	}

	return MachineType{
		Name:    "unknown",
		VCPU:    1,
		Memory:  1,
		GPU:     0,
		GPUType: "none",
	}
}

type InstancesExplorer struct {
	*Explorer
	client       *compute.InstancesClient
	machineTypes MachineTypes
	mu           *sync.Mutex
}

func NewInstancesExplorer(ctx context.Context, explorer *Explorer) (instanceExplorer *InstancesExplorer, err error) {
	instanceExplorer = &InstancesExplorer{
		Explorer: explorer,
		mu:       new(sync.Mutex),
	}

	instanceExplorer.client, err = compute.NewInstancesRESTClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create compute instances rest client: %w", err)
	}

	instanceExplorer.loadMachineTypes()

	return instanceExplorer, nil
}

func (instanceExplorer *InstancesExplorer) loadMachineTypes() {
	err := json.NewDecoder(bytes.NewReader(machinetypes.JSONFile)).Decode(&instanceExplorer.machineTypes)
	must.NoError(err)
}

func (instanceExplorer *InstancesExplorer) collectMetrics(ctx context.Context, zone string, metrics chan *cloudcarbonexporter.Metric) error {
	instancesIter := instanceExplorer.client.List(ctx, &computepb.ListInstancesRequest{
		Project: instanceExplorer.projectID,
		Zone:    zone,
	})

	for {
		instance, err := instancesIter.Next()
		if err == iterator.Done {
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

		watts := processor.EstimatePowerUsageWithTDP(machineType.VCPU, cpuUsage)
		watts += primitives.EstimateMemoryPowerUsage(machineType.Memory)
		for _, disk := range instance.Disks {
			// Physical disks (SCRATCH) are directly attached to the instance
			if *disk.Type == "SCRATCH" {
				watts += primitives.EstimateLocalSSDPowerUsage(1)
			}
		}

		metrics <- &cloudcarbonexporter.Metric{
			Name: "estimated_watts",
			Labels: cloudcarbonexporter.MergeLabels(
				map[string]string{
					"kind":          "compute/Instance",
					"instance_name": instanceName,
					"zone":          lastURLPathFragment(instance.GetZone()),
					"region":        instanceExplorer.zones.GetRegion(lastURLPathFragment(instance.GetZone())),
				},
				instance.Labels,
			),
			Value: watts,
		}
	}

	return nil
}

func (instanceExplorer *InstancesExplorer) GetInstanceAverageCPULoad(ctx context.Context, instanceName string) (float64, error) {
	// locking mutex prevents monitoring requests sent in parallel
	instanceExplorer.mu.Lock()
	defer instanceExplorer.mu.Unlock()

	key := "instances_average_cpu"
	entry, err := instanceExplorer.cache.GetOrSet(ctx, key, func(ctx context.Context) (any, error) {
		return instanceExplorer.ListInstanceCPUAverage(ctx)
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
func (explorer *InstancesExplorer) ListInstanceCPUAverage(ctx context.Context) (map[string]float64, error) {
	promqlExpression := `avg by (instance_name)(rate(compute_googleapis_com:instance_cpu_usage_time{monitored_resource="gce_instance"}[5m]))`
	period := 10 * time.Minute

	instanceList, err := explorer.query(ctx, promqlExpression, "instance_name", period)
	if err != nil {
		return nil, fmt.Errorf("failed to query for instance monitoring data: %w", err)
	}

	return instanceList, nil
}

type DisksExplorer struct {
	*Explorer
	client *compute.DisksClient
}

func NewDisksExplorer(ctx context.Context, explorer *Explorer) (disksExplorer *DisksExplorer, err error) {
	disksExplorer = &DisksExplorer{
		Explorer: explorer,
	}

	disksExplorer.client, err = compute.NewDisksRESTClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create disks rest client: %w", err)
	}

	return disksExplorer, nil
}

func (disksExplorer *DisksExplorer) collectMetrics(ctx context.Context, zone string, metrics chan *cloudcarbonexporter.Metric) error {
	disksIter := disksExplorer.client.List(ctx, &computepb.ListDisksRequest{
		Project: disksExplorer.projectID,
		Zone:    zone,
	})

	for {
		disk, err := disksIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to iterate on next disk: %w", err)
		}

		diskName := disk.GetName()
		fmt.Println("got disk", diskName, zone)

		watts := 0.0
		switch lastURLPathFragment(disk.GetType()) {
		case "pd-standard":
			watts = cloud.EstimateHDDBlockStorage(float64(*disk.SizeGb))
		default:
			watts = cloud.EstimateSSDBlockStorage(float64(*disk.SizeGb))
		}
		replicas := 1
		if len(disk.ReplicaZones) > 0 {
			replicas += len(disk.ReplicaZones)
		}

		watts = watts * float64(replicas)

		metrics <- &cloudcarbonexporter.Metric{
			Name: "estimated_watts",
			Labels: cloudcarbonexporter.MergeLabels(
				map[string]string{
					"kind":      "compute/Disk",
					"disk_name": diskName,
					"zone":      lastURLPathFragment(disk.GetZone()),
					"region":    disksExplorer.zones.GetRegion(lastURLPathFragment(disk.GetZone())),
				},
				disk.Labels,
			),
			Value: watts,
		}
	}

	return nil
}

type RegionDisksExplorer struct {
	*Explorer
	client *compute.RegionDisksClient
}

func NewRegionDisksExplorer(ctx context.Context, explorer *Explorer) (disksExplorer *RegionDisksExplorer, err error) {
	disksExplorer = &RegionDisksExplorer{
		Explorer: explorer,
	}

	disksExplorer.client, err = compute.NewRegionDisksRESTClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create region disks rest client: %w", err)
	}

	return disksExplorer, nil
}

func (disksExplorer *RegionDisksExplorer) collectMetrics(ctx context.Context, region string, metrics chan *cloudcarbonexporter.Metric) error {
	if region == "global" {
		return nil
	}

	regionDisksIter := disksExplorer.client.List(ctx, &computepb.ListRegionDisksRequest{
		Project: disksExplorer.projectID,
		Region:  region,
	})

	for {
		disk, err := regionDisksIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to iterate on next disk: %w", err)
		}

		diskName := disk.GetName()
		fmt.Println("got region disk", diskName, region)

		watts := 0.0
		switch lastURLPathFragment(disk.GetType()) {
		case "pd-standard":
			watts = cloud.EstimateHDDBlockStorage(float64(*disk.SizeGb))
		default:
			watts = cloud.EstimateSSDBlockStorage(float64(*disk.SizeGb))
		}

		replicas := 2

		watts = watts * float64(replicas)

		metrics <- &cloudcarbonexporter.Metric{
			Name: "estimated_watts",
			Labels: cloudcarbonexporter.MergeLabels(
				map[string]string{
					"kind":      "compute/RegionDisk",
					"disk_name": diskName,
					"region":    region,
				},
				disk.Labels,
			),
			Value: watts,
		}
	}

	return nil
}
