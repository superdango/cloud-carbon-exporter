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
	defaultTimeout time.Duration
	collector      *Collector
}

// NewOpenMetricsHandler create a new OpenMetricsHandler
func NewOpenMetricsHandler(collector *Collector) *OpenMetricsHandler {
	return &OpenMetricsHandler{
		defaultTimeout: 10 * time.Second,
		collector:      collector,
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
	errgctx, cancel := context.WithTimeout(errgctx, rh.defaultTimeout)
	defer cancel()

	errg.Go(func() error {
		rh.collector.Collect(errgctx, metrics)
		return nil
	})

	errg.Go(func() error {
		return writeMetrics(errgctx, w, metrics)
	})

	err := errg.Wait()
	if err != nil {
		slog.Error("failed to collect metrics", "err", err.Error(), traceAttr)
		http.Error(w, err.Error(), 500)
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
		}
	}
}
