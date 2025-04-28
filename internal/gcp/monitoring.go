package gcp

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"strconv"
	"time"

	"github.com/superdango/cloud-carbon-exporter/internal/must"

	"github.com/mitchellh/mapstructure"
	"google.golang.org/api/monitoring/v1"
)

// query the monitoring api and returns standardized cloud carbon metric
func (explorer *Explorer) query(ctx context.Context, promql string, resourceName string, resolution time.Duration) (map[string]float64, error) {
	body, err := explorer.monitoringClient.Projects.Location.Prometheus.Api.V1.QueryRange("projects/"+explorer.ProjectID, "global", &monitoring.QueryRangeRequest{
		Start: time.Now().Add(-resolution).Format(time.RFC3339),
		End:   time.Now().Format(time.RFC3339),
		Step:  resolution.String(),
		Query: promql,
	}).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to send promql query range request (%s): %w", promql, err)
	}

	queryResponse := new(promQueryResponse)
	if err := mapstructure.Decode(body.Data, queryResponse); err != nil {
		return nil, err
	}

	metrics := make(map[string]float64, len(queryResponse.Result))
	for _, result := range queryResponse.Result {
		resourceName, found := result.Metric[resourceName]
		if !found {
			slog.Warn("abandoning metric, cannot extract resource name in labels", "resourceName", resourceName)
			continue
		}

		_, metrics[resourceName] = result.valueAt(result.len() - 1)
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
