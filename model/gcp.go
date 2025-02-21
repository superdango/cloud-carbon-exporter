package model

import (
	"embed"
	"encoding/csv"
	"strconv"

	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
	"github.com/superdango/cloud-carbon-exporter/internal/must"
)

type GoogleCloudPlatformModel struct {
	carbonIntensity cloudcarbonexporter.CarbonIntensityMap
	calculations map[string]func(r *cloudcarbonexporter.Resource) []cloudcarbonexporter.Metric
}

func NewGoogleCloudPlatform() *GoogleCloudPlatformModel {
	carbonIntensity := NewGCPCarbonIntensityMap()

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

// generateResourceMetrics returns watts metrics and related emissions from a watts value.
func generateResourceMetrics(resource *cloudcarbonexporter.Resource, watts float64, intensity cloudcarbonexporter.CarbonIntensityMap) []cloudcarbonexporter.Metric {
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
	emissions := intensity.ComputeCO2eq(wattsMetric)
	return []cloudcarbonexporter.Metric{wattsMetric, emissions}
}

//go:embed data/gcp_region_carbon_info_2023.csv
var carboninfo embed.FS

// NewGCPCarbonIntensityMap loads and parse official carbon data provided by GCP
// https://github.com/GoogleCloudPlatform/region-carbon-info
// Should be updated each year.
func NewGCPCarbonIntensityMap() cloudcarbonexporter.CarbonIntensityMap {
	f, err := carboninfo.Open("data/gcp_region_carbon_info_2023.csv")
	must.NoError(err)

	intensityData := csv.NewReader(f)
	locations, err := intensityData.ReadAll()
	must.NoError(err)

	intensity := make(cloudcarbonexporter.CarbonIntensityMap)

	for line, location := range locations {
		// skip csv header
		if line == 0 {
			continue
		}
		must.Assert(len(location) == 4, "csv line must be 4 fields length")
		region := location[0]
		co2eqbykwh := strToFloat64(location[3])

		intensity[region] = co2eqbykwh / 60 / 60 / 1000
	}

	intensity["emea"] = intensity.Average([]string{"eu", "me", "af"}...)
	intensity["apac"] = intensity.Average([]string{"as", "au"}...)
	intensity["amer"] = intensity.Average([]string{"no", "so", "us"}...)
	intensity["global"] = intensity.Average("emea", "apac", "amer")

	return intensity
}

func strToFloat64(s string) float64 {
	f, err := strconv.ParseFloat(s, 64)
	must.NoError(err)
	return f
}

// return models{
// 	assets: map[string]func(resource cloudcarbonexporter.Resource) cloudcarbonexporter.Metric{
// 		"compute.googleapis.com/Disk": func(resource cloudcarbonexporter.Resource) cloudcarbonexporter.Metric {
// 			watts := 0.0

// 			if sizeGb, found := resource.Source["sizeGb"]; found {
// 				gb, err := strconv.Atoi(fmt.Sprintf("%s", sizeGb))
// 				if err != nil {
// 					slog.Warn("sizeGb not found of bad format, this should not happen. value defaulted to 10", "resource_kind", resource.Kind, "sizgGbValue", sizeGb)
// 					gb = 10 // Gb
// 				}
// 				watts = float64(gb) * 0.1
// 			}

// 			if storageType, found := resource.Source["type"]; found {
// 				switch filepath.Base(fmt.Sprintf("%s", storageType)) {
// 				case "pd-standard":
// 					watts *= 5.2
// 				}
// 				resource.Labels["storage_type"] = filepath.Base(fmt.Sprintf("%s", storageType))
// 			}

// 			return cloudcarbonexporter.Metric{
// 				ResourceID: resource.ID,
// 				Name:       "estimated_watts",
// 				Value:      watts,
// 				Labels: cloudcarbonexporter.MergeLabels(resource.Labels, map[string]string{
// 					"model_version": getModelsVersion(),
// 					"location":      resource.Region,
// 					"resource_kind": resource.Kind,
// 					"metric_id":     "disk_provisioned_size",
// 				}),
// 			}
// 		},
// 		"compute.googleapis.com/RegionDisk": func(resource cloudcarbonexporter.Resource) cloudcarbonexporter.Metric {
// 			// region disk consumes two zonal disks + synchronization network
// 			metric := getModels().assets["compute.googleapis.com/Disk"](resource)
// 			metric.Value *= 2.1
// 			return metric
// 		},
// 	},
// 	monitoring: map[string]model{
// 		"serviceruntime.googleapis.com/api": {
// 			"googleapis_request_count": {
// 				resourceNameField: "service",
// 				query:             `avg by (service,location)(rate(serviceruntime_googleapis_com:api_request_count{monitored_resource="consumed_api"}[5m]))`,
// 				wattConverter: func(metric cloudcarbonexporter.Metric) cloudcarbonexporter.Metric {
// 					metric.Value = metric.Value / 100_000
// 					return metric
// 				},
// 			},
// 		},

// 		"storage.googleapis.com/Bucket": {
// 			"bucket_total_bytes": {
// 				resourceNameField: "bucket_name",
// 				query:             `avg by (bucket_name,location,storage_class,type)(avg_over_time(storage_googleapis_com:storage_v2_total_bytes{monitored_resource="gcs_bucket"}[5m]))`,
// 				wattConverter: func(metric cloudcarbonexporter.Metric) cloudcarbonexporter.Metric {
// 					metric.Value = metric.Value / 1_000_000
// 					return metric
// 				},
// 			},
// 			"bucket_request_count": {
// 				resourceNameField: "bucket_name",
// 				query:             `avg by (bucket_name,location)(rate(storage_googleapis_com:api_request_count{monitored_resource="gcs_bucket"}[5m]))`,
// 				wattConverter: func(metric cloudcarbonexporter.Metric) cloudcarbonexporter.Metric {
// 					metric.Value = metric.Value / 100_000
// 					return metric
// 				},
// 			},
// 		},

// 		"compute.googleapis.com/Instance": {
// 			"instance_cpu_usage_time": {
// 				resourceNameField: "instance_name",
// 				query:             `avg by (instance_name)(rate(compute_googleapis_com:instance_cpu_usage_time{monitored_resource="gce_instance"}[5m]))`, // cpu/second
// 				wattConverter: func(metric cloudcarbonexporter.Metric) cloudcarbonexporter.Metric {
// 					return metric
// 				},
// 			},
// 			"instance_cpu_reserved_cores": {
// 				resourceNameField: "instance_name",
// 				query:             `sum by (instance_name)(avg_over_time(compute_googleapis_com:instance_cpu_reserved_cores{monitored_resource="gce_instance"}[5m]))`,
// 				wattConverter: func(metric cloudcarbonexporter.Metric) cloudcarbonexporter.Metric {
// 					metric.Value = metric.Value * 10
// 					return metric
// 				},
// 			},
// 		},

// 		"run.googleapis.com/Service": {
// 			"container_cpu_allocation_time": {
// 				resourceNameField: "service_name",
// 				query:             `sum by (service_name)(rate(run_googleapis_com:container_cpu_allocation_time{monitored_resource="cloud_run_revision"}[5m]))`, //cpu/second
// 				wattConverter: func(metric cloudcarbonexporter.Metric) cloudcarbonexporter.Metric {
// 					return metric
// 				},
// 			},
// 		},
// 	},
// }
