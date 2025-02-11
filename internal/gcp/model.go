package gcp

import (
	"cloudcarbonexporter"
	"cloudcarbonexporter/internal/must"
	"fmt"
	"log/slog"
	"path/filepath"
	"strconv"
)

// models holds every calculation methods for all resource kind and metrics. If signals
// comes from monitoring api then use "monitoring" models. If resource is directly coming
// from Asset inventory then use "assets" models
type models struct {
	monitoring map[string]model
	assets     map[string]func(resource cloudcarbonexporter.Resource) cloudcarbonexporter.Metric
}

// model holds the calculation methods for a monitored resource
type model map[string]signal

// signal is used to convert measurement from a resource into watts
type signal struct {
	resourceNameField string
	query             string
	wattConverter     func(metric cloudcarbonexporter.Metric) cloudcarbonexporter.Metric
}

// getModelsVersion returns the current model versions
func getModelsVersion() string {
	return "v0.0.1"
}

// getModels returns the current model
func getModels() models {
	return models{
		assets: map[string]func(resource cloudcarbonexporter.Resource) cloudcarbonexporter.Metric{
			"compute.googleapis.com/Disk": func(resource cloudcarbonexporter.Resource) cloudcarbonexporter.Metric {
				watts := 0.0

				if sizeGb, found := resource.Source["sizeGb"]; found {
					gb, err := strconv.Atoi(fmt.Sprintf("%s", sizeGb))
					if err != nil {
						slog.Warn("sizeGb not found of bad format, this should not happen. value defaulted to 10", "resource_kind", resource.Kind, "sizgGbValue", sizeGb)
						gb = 10 // Gb
					}
					watts = float64(gb) * 0.1
				}

				if storageType, found := resource.Source["type"]; found {
					switch filepath.Base(fmt.Sprintf("%s", storageType)) {
					case "pd-standard":
						watts *= 5.2
					}
					resource.Labels["storage_type"] = filepath.Base(fmt.Sprintf("%s", storageType))
				}

				return cloudcarbonexporter.Metric{
					ResourceName: resource.Name,
					Name:         "estimated_watts",
					Value:        watts,
					Labels: cloudcarbonexporter.MergeLabels(resource.Labels, map[string]string{
						"model_version": getModelsVersion(),
						"location":      resource.Location,
						"resource_kind": resource.Kind,
						"metric_id":     "disk_provisioned_size",
					}),
				}
			},
			"compute.googleapis.com/RegionDisk": func(resource cloudcarbonexporter.Resource) cloudcarbonexporter.Metric {
				// region disk consumes two zonal disks + synchronization network
				metric := getModels().assets["compute.googleapis.com/Disk"](resource)
				metric.Value *= 2.1
				return metric
			},
		},
		monitoring: map[string]model{
			"serviceruntime.googleapis.com/api": {
				"googleapis_request_count": {
					resourceNameField: "service",
					query:             `avg by (service,location)(rate(serviceruntime_googleapis_com:api_request_count{monitored_resource="consumed_api"}[5m]))`,
					wattConverter: func(metric cloudcarbonexporter.Metric) cloudcarbonexporter.Metric {
						metric.Value = metric.Value / 100_000
						return metric
					},
				},
			},

			"storage.googleapis.com/Bucket": {
				"bucket_total_bytes": {
					resourceNameField: "bucket_name",
					query:             `avg by (bucket_name,location,storage_class,type)(avg_over_time(storage_googleapis_com:storage_v2_total_bytes{monitored_resource="gcs_bucket"}[5m]))`,
					wattConverter: func(metric cloudcarbonexporter.Metric) cloudcarbonexporter.Metric {
						metric.Value = metric.Value / 1_000_000
						return metric
					},
				},
				"bucket_request_count": {
					resourceNameField: "bucket_name",
					query:             `avg by (bucket_name,location)(rate(storage_googleapis_com:api_request_count{monitored_resource="gcs_bucket"}[5m]))`,
					wattConverter: func(metric cloudcarbonexporter.Metric) cloudcarbonexporter.Metric {
						metric.Value = metric.Value / 100_000
						return metric
					},
				},
			},

			"compute.googleapis.com/Instance": {
				"instance_cpu_usage_time": {
					resourceNameField: "instance_name",
					query:             `avg by (instance_name)(rate(compute_googleapis_com:instance_cpu_usage_time{monitored_resource="gce_instance"}[5m]))`, // cpu/second
					wattConverter: func(metric cloudcarbonexporter.Metric) cloudcarbonexporter.Metric {
						return metric
					},
				},
				"instance_cpu_reserved_cores": {
					resourceNameField: "instance_name",
					query:             `sum by (instance_name)(avg_over_time(compute_googleapis_com:instance_cpu_reserved_cores{monitored_resource="gce_instance"}[5m]))`,
					wattConverter: func(metric cloudcarbonexporter.Metric) cloudcarbonexporter.Metric {
						metric.Value = metric.Value * 10
						return metric
					},
				},
			},

			"run.googleapis.com/Service": {
				"container_cpu_allocation_time": {
					resourceNameField: "service_name",
					query:             `sum by (service_name)(rate(run_googleapis_com:container_cpu_allocation_time{monitored_resource="cloud_run_revision"}[5m]))`, //cpu/second
					wattConverter: func(metric cloudcarbonexporter.Metric) cloudcarbonexporter.Metric {
						return metric
					},
				},
			},
		},
	}
}

// isSupportedRessource returns true if model exists for the resource kind
func (m models) isSupportedRessource(resourceKind string) bool {
	if _, ok := m.monitoring[resourceKind]; ok {
		return true
	}
	if _, ok := m.assets[resourceKind]; ok {
		return true
	}
	return false
}

// getResourceQueries returns all monitoring queries to send to retreive resource signals
func (m models) getResourceQueries(resourceKind string) map[string]string {
	queries := make(map[string]string)
	for resource, model := range m.monitoring {
		if resource == resourceKind {
			for query, signal := range model {
				queries[query] = signal.query
			}
		}
	}
	return queries
}

// wattMetricFunc return the function use to estimate watts from a resource metric
func (m models) wattMetricFunc(resourceKind string, metricID string) func(value cloudcarbonexporter.Metric) cloudcarbonexporter.Metric {
	for resource, model := range m.monitoring {
		if resource == resourceKind {
			for metric, signal := range model {
				if metric == metricID {
					return signal.wattConverter
				}
			}
		}
	}
	must.Fail(fmt.Sprintf("no transformer available for the resource kind: %s and metric id: %s", resourceKind, metricID))
	return nil
}

// getResourceNameField return the metric field containing the resource name
func (m models) getResourceNameField(resourceKind string, metricID string) string {
	for resource, model := range m.monitoring {
		if resource == resourceKind {
			for metric, signal := range model {
				if metric == metricID {
					return signal.resourceNameField
				}
			}
		}
	}
	must.Fail(fmt.Sprintf("no resource name available for the resource kind: %s and metric id: %s", resourceKind, metricID))
	return ""
}
