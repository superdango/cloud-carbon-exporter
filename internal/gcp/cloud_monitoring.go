package gcp

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"strconv"
	"time"

	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
	"github.com/superdango/cloud-carbon-exporter/internal/must"

	"github.com/mitchellh/mapstructure"
	"google.golang.org/api/monitoring/v1"
)

// monitoringService wraps the Google Monitoring API and use the Managed Prometheus API
type monitoringService struct {
	*monitoring.Service
	projectID  string
	resolution time.Duration
}

// newMonitoringService returns a new monitoring service
func newMonitoringService(ctx context.Context, projectID string) (*monitoringService, error) {
	var err error
	gcpmonitoring := &monitoringService{
		projectID:  "projects/" + projectID,
		resolution: 5 * time.Minute,
	}
	gcpmonitoring.Service, err = monitoring.NewService(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize gcp monitoring: %w", err)
	}

	return gcpmonitoring, nil
}

// query the monitoring api and returns standardized cloud carbon metric
func (service *monitoringService) query(ctx context.Context, promql string, resourceName string) ([]cloudcarbonexporter.Metric, error) {
	body, err := service.Projects.Location.Prometheus.Api.V1.QueryRange(service.projectID, "global", &monitoring.QueryRangeRequest{
		Start: time.Now().Add(service.resolution * -1).Format(time.RFC3339),
		End:   time.Now().Format(time.RFC3339),
		Step:  service.resolution.String(),
		Query: promql,
	}).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to send promql query range request (%s): %w", promql, err)
	}

	queryResponse := new(promQueryResponse)
	if err := mapstructure.Decode(body.Data, queryResponse); err != nil {
		return nil, err
	}

	slog.Debug("decoded promquery response", "num_results", len(queryResponse.Result))

	metrics := make([]cloudcarbonexporter.Metric, len(queryResponse.Result))
	for i, result := range queryResponse.Result {
		resourceName, found := result.Metric[resourceName]
		if !found {
			slog.Warn("abandoning metric, cannot extract resource name in labels", "resourceName", resourceName)
			continue
		}

		_, value := result.valueAt(result.len() - 1)
		metrics[i] = cloudcarbonexporter.Metric{
			Labels:     result.Metric,
			Value:      value,
			ResourceID: resourceName,
		}
	}

	return metrics, nil
}

// promQueryResponse represents the query response of the managed prometheus api
type promQueryResponse struct {
	Result     []promQueryResponseResult `json:"result"`
	ResultType string                    `json:"resultType"`
}

// promQueryResponseResult olds metric labels and timestamped values
type promQueryResponseResult struct {
	Metric map[string]string `json:"metric"`
	Values [][]any           `json:"values"`
}

// len of the prometheus response
func (r *promQueryResponseResult) len() int {
	return len(r.Values)
}

// valueAt returns the timestamp and the value located at index
func (r *promQueryResponseResult) valueAt(index int) (unixTimestamp float64, value float64) {
	must.Assert(index >= 0 && index < r.len(), fmt.Sprintf("invalid index value %d, result len=%d", index, r.len()))
	must.Assert(len(r.Values[index]) == 2, "response result item must have 2 data: timestamp and float")

	timestamp, ok := r.Values[index][0].(float64)
	if !ok {
		slog.Warn("first item in response value is not a float64", "original_type", reflect.TypeOf(r.Values[index][0]))
		return 0, 0
	}

	stringValue, ok := r.Values[index][1].(string)
	if !ok {
		slog.Warn("second item in response value is not a string", "original_type", reflect.TypeOf(r.Values[index][1]))
		return 0, 0
	}

	value, err := strconv.ParseFloat(stringValue, 64)
	if err != nil {
		slog.Warn("second item in response value cannot be converted to float64", "err", err, "original_value", value)
		return 0, 0
	}

	return timestamp, value
}
