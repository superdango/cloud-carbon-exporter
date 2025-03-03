package cloudcarbonfootprint

import (
	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
	"github.com/superdango/cloud-carbon-exporter/model/carbon"
)

type GoogleCloudPlatformModel struct {
	carbonIntensity cloudcarbonexporter.CarbonIntensityMap
	calculations    map[string]func(r *cloudcarbonexporter.Resource) []cloudcarbonexporter.Metric
}

func NewGoogleCloudPlatform() *GoogleCloudPlatformModel {
	carbonIntensity := carbon.NewGCPCarbonIntensityMap()

	generateResourceMetrics := func(resource *cloudcarbonexporter.Resource, watts float64) []cloudcarbonexporter.Metric {
		wattsMetric := cloudcarbonexporter.Metric{
			Name:  "estimated_watts",
			Value: watts,
			Labels: cloudcarbonexporter.MergeLabels(resource.Labels, map[string]string{
				"model_version":  "0",
				"cloud_provider": "gcp",
				"region":         resource.Region,
				"resource_id":    resource.ID,
				"resource_kind":  resource.Kind,
			}),
		}
		emissions := carbonIntensity.ComputeCO2eq(wattsMetric)
		return []cloudcarbonexporter.Metric{wattsMetric, emissions}
	}

	return &GoogleCloudPlatformModel{
		carbonIntensity: carbonIntensity,
		calculations: map[string]func(r *cloudcarbonexporter.Resource) []cloudcarbonexporter.Metric{
			"compute.googleapis.com/Instance": func(r *cloudcarbonexporter.Resource) []cloudcarbonexporter.Metric {
				return generateResourceMetrics(r, 10.0)
			},
		},
	}
}

func (gcp *GoogleCloudPlatformModel) Supports(r *cloudcarbonexporter.Resource) bool {
	if r.CloudProvider != "gcp" {
		return false
	}

	_, found := gcp.calculations[r.Kind]

	return found
}

func (gcp *GoogleCloudPlatformModel) ComputeMetrics(r *cloudcarbonexporter.Resource) []cloudcarbonexporter.Metric {
	if !gcp.Supports(r) {
		return nil
	}

	for kind, calculation := range gcp.calculations {
		if kind == r.Kind {
			return calculation(r)
		}
	}

	return nil
}
