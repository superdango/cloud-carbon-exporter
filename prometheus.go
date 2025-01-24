package cloudcarbonexporter

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"golang.org/x/sync/errgroup"
)

type PrometheusMetricsHandler struct {
	collectors []Collector
}

func NewHTTPMetricsHandler(collectors ...Collector) *PrometheusMetricsHandler {
	return &PrometheusMetricsHandler{
		collectors: collectors,
	}
}

func (rh *PrometheusMetricsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
		return OpenMetricsFormat(errgctx, w, metrics)
	})

	err := errg.Wait()
	if err != nil {
		slog.Error("failed to collect metrics", "err", err.Error(), traceAttr)
		w.WriteHeader(500)
		w.Write([]byte(err.Error()))
	}
	slog.Info("metrics have been successfully collected", traceAttr)
}

func OpenMetricsFormat(ctx context.Context, w io.Writer, metrics chan Metric) error {
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
