package cloudcarbonexporter

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
)

// OpenMetricsHandler implements the http.Handler interface
type OpenMetricsHandler struct {
	collectors []Collector
}

// NewOpenMetricsHandler create a new OpenMetricsHandler
func NewOpenMetricsHandler(collectors ...Collector) *OpenMetricsHandler {
	return &OpenMetricsHandler{
		collectors: collectors,
	}
}

// ServeHTTP implements the http.Handler interface. It collects all metrics from the configured
// collector and return them, formatted in the http response.
func (rh *OpenMetricsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	metrics := make(chan Metric)

	traceAttr := slog.Attr{}
	if traceID := r.Header.Get("X-Cloud-Trace-Context"); traceID != "" {
		traceAttr = slog.String("logging.googleapis.com/trace", traceID)
	}

	errg, errgctx := errgroup.WithContext(r.Context())

	for _, collector := range rh.collectors {
		collector := collector
		errg.Go(func() error {
			return collector.Collect(errgctx, metrics)
		})
	}

	errg.Go(func() error {
		return writeMetrics(errgctx, w, metrics)
	})

	err := errg.Wait()
	if err != nil {
		slog.Error("failed to collect metrics", "err", err.Error(), traceAttr)
		w.WriteHeader(500)
		w.Write([]byte(err.Error()))
		return
	}

	slog.Info("metrics have been successfully collected", traceAttr, "duration_ms", time.Since(start).Milliseconds())
}

// writeMetrics write all metrics sent over the channel and write them on the writer.
// Metrics labels are sorted lexicographically before being written.
func writeMetrics(ctx context.Context, w io.Writer, metrics chan Metric) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case metric, ok := <-metrics:
			if !ok {
				return nil
			}

			labels := make([]string, 0, len(metric.Labels))
			for labelName, labelValue := range metric.Labels {
				labels = append(labels, fmt.Sprintf(`%s="%s"`, labelName, labelValue))
			}
			slices.SortFunc(labels, strings.Compare)

			_, err := fmt.Fprintf(w, "%s{%s} %f\n", metric.Name, strings.Join(labels, ","), metric.Value)
			if err != nil {
				return fmt.Errorf("writing metric %s failed: %w", metric.Name, err)
			}
			slog.Debug("metric successfully written on writer", "metric", metric.Name)
		}
	}
}
