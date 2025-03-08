package boavizta

import (
	"embed"
	"encoding/json"
	"log/slog"
	"strings"

	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
	"github.com/superdango/cloud-carbon-exporter/internal/must"
	"github.com/superdango/cloud-carbon-exporter/model"
	"github.com/superdango/cloud-carbon-exporter/model/carbon"
)

//go:embed primitives/scaleway.json
var scalewayPrimitivesFS embed.FS

func NewScalewayModel() *model.Model {
	scwPrimitivesFile, err := scalewayPrimitivesFS.Open("primitives/scaleway.json")
	must.NoError(err)

	primitives := model.Primitives{}
	err = json.NewDecoder(scwPrimitivesFile).Decode(&primitives)
	must.NoError(err)

	carbonIntensity := carbon.NewScalewayCloudCarbonFootprintIntensityMap()
	generateResourceMetrics := func(resource *cloudcarbonexporter.Resource, watts float64) []cloudcarbonexporter.Metric {
		wattsMetric := cloudcarbonexporter.Metric{
			Name:  "estimated_watts",
			Value: watts,
			Labels: cloudcarbonexporter.MergeLabels(resource.Labels, map[string]string{
				"model_version":  "0",
				"cloud_provider": "scw",
				"region":         resource.Region,
				"resource_id":    resource.ID,
				"resource_kind":  resource.Kind,
			}),
		}
		emissions := carbonIntensity.ComputeCO2eq(wattsMetric)
		return []cloudcarbonexporter.Metric{wattsMetric, emissions}
	}

	return &model.Model{
		Provider:        "scw",
		CarbonIntensity: carbonIntensity,
		Calculations: map[string]func(r *cloudcarbonexporter.Resource) []cloudcarbonexporter.Metric{
			"instance": func(r *cloudcarbonexporter.Resource) []cloudcarbonexporter.Metric {
				watts := 3.0
				instanceType := strings.ToLower(r.Labels["instance_type"])
				loadTable, found := primitives[instanceType]
				if !found {
					slog.Warn("instance type not found", "provider", r.CloudProvider, "instance_type", instanceType)
					return generateResourceMetrics(r, watts)
				}

				watts = loadTable[50]
				return generateResourceMetrics(r, watts)
			},
		},
	}
}
