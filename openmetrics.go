package cloudcarbonexporter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
)

// OpenMetricsHandler implements the http.Handler interface
type OpenMetricsHandler struct {
	defaultTimeout time.Duration
	explorer       Explorer
}

// NewOpenMetricsHandler create a new OpenMetricsHandler
func NewOpenMetricsHandler(explorer Explorer) *OpenMetricsHandler {
	return &OpenMetricsHandler{
		defaultTimeout: 10 * time.Second,
		explorer:       explorer,
	}
}

// ServeHTTP implements the http.Handler interface. It collects all metrics from the configured
// collector and return them, formatted in the http response.
func (rh *OpenMetricsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	metrics := make(chan *Metric)
	errs := make(chan error)
	errCount := 0

	traceAttr := slog.Attr{}
	if traceID := r.Header.Get("X-Cloud-Trace-Context"); traceID != "" {
		traceAttr = slog.String("logging.googleapis.com/trace", traceID)
	}

	errg, errgctx := errgroup.WithContext(r.Context())
	errgctx, cancel := context.WithTimeout(errgctx, rh.defaultTimeout)
	defer cancel()

	errg.Go(func() error {
		defer close(metrics)
		defer close(errs)
		rh.explorer.CollectMetrics(errgctx, metrics, errs)

		metrics <- &Metric{
			Name: "collect_duration_ms",
			Labels: map[string]string{
				"action": "collect",
			},
			Value: float64(time.Since(start).Milliseconds()),
		}

		metrics <- &Metric{
			Name: "error_count",
			Labels: map[string]string{
				"action": "collect",
			},
			Value: float64(errCount),
		}

		return nil
	})

	errg.Go(func() error {
		for err := range errs {
			if err == nil {
				continue
			}

			errCount++

			experr := new(ExplorerErr)
			if errors.As(err, &experr) {
				slog.Warn("metrics collection failed", "err", experr, "op", experr.Operation)
				continue
			}
			slog.Warn("metrics collection failed", "err", err.Error())
		}

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
func writeMetrics(ctx context.Context, w io.Writer, metrics chan *Metric) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case metric, ok := <-metrics:
			if !ok {
				return nil
			}

			if metric == nil {
				slog.Warn("discarding nil metric")
				continue
			}
			if err := writeMetric(w, metric); err != nil {
				return fmt.Errorf("failed to write metric on writer: %w", err)
			}
		}
	}
}

func writeMetric(w io.Writer, metric *Metric) error {
	// sort labels in lexicographical order
	labels := make([]string, 0, len(metric.Labels))
	for labelName, labelValue := range metric.Labels {
		labels = append(labels, fmt.Sprintf(`%s="%s"`, labelName, labelValue))
	}
	slices.SortFunc(labels, strings.Compare)

	_, err := fmt.Fprintf(w, "%s{%s} %f\n", metric.Name, strings.Join(labels, ","), metric.Value)
	if err != nil {
		return fmt.Errorf("writing metric %s failed: %w", metric.Name, err)
	}

	return nil
}

// Metric olds the name and value of a measurement in addition to its labels.
type Metric struct {
	Name   string
	Labels map[string]string
	Value  float64
}

// Clone return a deep copy of a metric.
func (m Metric) Clone() Metric {
	copiedLabel := make(map[string]string, len(m.Labels))
	maps.Copy(copiedLabel, m.Labels)
	return Metric{
		Name:   m.Name,
		Value:  m.Value,
		Labels: copiedLabel,
	}
}

func (m *Metric) SetLabel(key, value string) *Metric {
	m.Labels = MergeLabels(
		m.Labels,
		map[string]string{
			key: value,
		},
	)
	return m
}

func (m Metric) SanitizeLabels() Metric {
	newLabels := make(map[string]string)
	invalidChars := []string{".", "/", "-", ":", ";"}
	for label, value := range m.Labels {
		for _, char := range invalidChars {
			label = strings.ReplaceAll(label, char, "_")
		}
		newLabels[label] = value
	}
	m.Labels = newLabels
	return m
}
