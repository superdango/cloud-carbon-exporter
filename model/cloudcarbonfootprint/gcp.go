package cloudcarbonfootprint

import (
	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
	"github.com/superdango/cloud-carbon-exporter/model"
	"github.com/superdango/cloud-carbon-exporter/model/carbon"
)

func NewGoogleCloudPlatformModel() *model.Model {
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

	return &model.Model{
		CarbonIntensity: carbonIntensity,
		Calculations: map[string]func(r *cloudcarbonexporter.Resource) []cloudcarbonexporter.Metric{
			"compute.googleapis.com/Instance": func(r *cloudcarbonexporter.Resource) []cloudcarbonexporter.Metric {
				return generateResourceMetrics(r, 10.0)
			},
		},
	}
}
