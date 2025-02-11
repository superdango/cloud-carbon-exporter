package aws

import (
	"cloudcarbonexporter"
	"cloudcarbonexporter/internal/must"
	"fmt"
)

// models holds every calculation methods for all resource kind and metrics. If signals
// comes from monitoring api then use "monitoring" models. If resource is directly coming
// from Asset inventory then use "assets" models
type models struct {
	monitoring map[string]model
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
		monitoring: map[string]model{
			"ec2/instance": {
				"cpu_credit_usage": {
					resourceNameField: "service",
					query:             `avg by (service,location)(rate(serviceruntime_googleapis_com:api_request_count{monitored_resource="consumed_api"}[5m]))`,
					wattConverter: func(metric cloudcarbonexporter.Metric) cloudcarbonexporter.Metric {
						metric.Value = metric.Value / 100_000
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
