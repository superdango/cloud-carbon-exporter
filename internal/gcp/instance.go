package gcp

import (
	"context"

	"cloud.google.com/go/asset/apiv1/assetpb"
	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
	"github.com/superdango/cloud-carbon-exporter/model/energy/primitives"
)

func (explorer *Explorer) instanceEnergyMetric(ctx context.Context, asset *assetpb.Asset, metrics chan *cloudcarbonexporter.Metric, errs chan error) {
	instanceName := asset.Resource.Data.GetFields()["name"].GetStringValue()
	cpuPlatform := asset.Resource.Data.GetFields()["cpuPlatform"].GetStringValue()
	processor := primitives.LookupProcessorByName(cpuPlatform)
	cpuUsage, err := explorer.GetInstanceCPUAverage(ctx, instanceName)
	if err != nil {
		errs <- err
		return
	}

	watts := processor.EstimatePowerUsageWithTDP(1, cpuUsage)
	watts += primitives.EstimateMemoryPowerUsage(4)

	metrics <- &cloudcarbonexporter.Metric{
		Name:       "estimated_watts",
		ResourceID: instanceName,
		Labels:     mapToStringMap(asset.Resource.Data.AsMap()["labels"]),
		Value:      watts,
	}
}
