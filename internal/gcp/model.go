package gcp

import (
	"cloudcarbonexporter"
	"cloudcarbonexporter/internal/must"
	"fmt"
	"path/filepath"
	"strconv"
)

type signal struct {
	resourceName  string
	query         string
	wattConverter transformer
}

type models struct {
	monitoring map[string]model
	assets     map[string]func(r cloudcarbonexporter.Resource) *cloudcarbonexporter.Metric
}
type model map[string]signal
type transformer func(metric cloudcarbonexporter.Metric) cloudcarbonexporter.Metric

func getModelsVersion() string {
	return "v0.0.1"
}

func getModels() models {
	return models{
		assets: map[string]func(r cloudcarbonexporter.Resource) *cloudcarbonexporter.Metric{
			"compute.googleapis.com/Disk": func(r cloudcarbonexporter.Resource) *cloudcarbonexporter.Metric {
				watts := 0.0

				if sizeGb, found := r.Source["sizeGb"]; found {
					gb, err := strconv.Atoi(fmt.Sprintf("%s", sizeGb))
					if err != nil {
						return nil
					}
					watts = float64(gb) * 0.1
				}

				if storageType, found := r.Source["type"]; found {
					switch filepath.Base(fmt.Sprintf("%s", storageType)) {
					case "pd-standard":
						watts *= 5.2
					}
					r.Labels["storage_type"] = filepath.Base(fmt.Sprintf("%s", storageType))
				}

				return &cloudcarbonexporter.Metric{
					ResourceName: "device_name",
					Name:         "estimated_watts",
					Value:        watts,
					Labels: mergeLabels(r.Labels, map[string]string{
						"model_version": getModelsVersion(),
						"location":      r.Location,
						"resource_kind": r.Kind,
						"metric_id":     "disk_provisioned_size",
					}),
				}
			},
			"compute.googleapis.com/RegionDisk": func(r cloudcarbonexporter.Resource) *cloudcarbonexporter.Metric {
				metric := getModels().assets["compute.googleapis.com/Disk"](r)
				metric.Value *= 2.1
				return metric
			},
		},
		monitoring: map[string]model{
			"serviceruntime.googleapis.com/api": {
				"googleapis_request_count": {
					resourceName: "service",
					query:        `avg by (service,location)(rate(serviceruntime_googleapis_com:api_request_count{monitored_resource="consumed_api"}[5m]))`,
					wattConverter: func(metric cloudcarbonexporter.Metric) cloudcarbonexporter.Metric {
						metric.Value = metric.Value / 100_000
						return metric
					},
				},
			},

			"storage.googleapis.com/Bucket": {
				"bucket_total_bytes": {
					resourceName: "bucket_name",
					query:        `avg by (bucket_name,location,storage_class,type)(avg_over_time(storage_googleapis_com:storage_v2_total_bytes{monitored_resource="gcs_bucket"}[5m]))`,
					wattConverter: func(metric cloudcarbonexporter.Metric) cloudcarbonexporter.Metric {
						metric.Value = metric.Value / 1_000_000
						return metric
					},
				},
				"bucket_request_count": {
					resourceName: "bucket_name",
					query:        `avg by (bucket_name,location)(rate(storage_googleapis_com:api_request_count{monitored_resource="gcs_bucket"}[5m]))`,
					wattConverter: func(metric cloudcarbonexporter.Metric) cloudcarbonexporter.Metric {
						metric.Value = metric.Value / 100_000
						return metric
					},
				},
			},

			"compute.googleapis.com/Instance": {
				"instance_cpu_usage_time": {
					resourceName: "instance_name",
					query:        `avg by (instance_name)(rate(compute_googleapis_com:instance_cpu_usage_time{monitored_resource="gce_instance"}[5m]))`, // cpu/second
					wattConverter: func(metric cloudcarbonexporter.Metric) cloudcarbonexporter.Metric {
						return metric
					},
				},
				"instance_cpu_reserved_cores": {
					resourceName: "instance_name",
					query:        `sum by (instance_name)(avg_over_time(compute_googleapis_com:instance_cpu_reserved_cores{monitored_resource="gce_instance"}[5m]))`,
					wattConverter: func(metric cloudcarbonexporter.Metric) cloudcarbonexporter.Metric {
						metric.Value = metric.Value * 10
						return metric
					},
				},
			},

			"run.googleapis.com/Service": {
				"container_cpu_allocation_time": {
					resourceName: "service_name",
					query:        `sum by (service_name)(rate(run_googleapis_com:container_cpu_allocation_time{monitored_resource="cloud_run_revision"}[5m]))`, //cpu/second
					wattConverter: func(metric cloudcarbonexporter.Metric) cloudcarbonexporter.Metric {
						return metric
					},
				},
			},
		},
	}
}

func (m models) isSupportedRessource(resourceKind string) bool {
	if _, ok := m.monitoring[resourceKind]; ok {
		return true
	}
	if _, ok := m.assets[resourceKind]; ok {
		return true
	}
	return false
}

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

func (m models) getWattTransformer(resourceKind string, metricID string) func(value cloudcarbonexporter.Metric) cloudcarbonexporter.Metric {
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

func (m models) getResourceNameField(resourceKind string, metricID string) string {
	for resource, model := range m.monitoring {
		if resource == resourceKind {
			for metric, signal := range model {
				if metric == metricID {
					return signal.resourceName
				}
			}
		}
	}
	must.Fail(fmt.Sprintf("no resource name available for the resource kind: %s and metric id: %s", resourceKind, metricID))
	return ""
}
