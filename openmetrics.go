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
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
)

type Ctx struct {
	context.Context
	calls *atomic.Int64
}

type Context interface {
	context.Context
	IncrCalls()
	Calls() int
}

func WrapCtx(ctx context.Context) Context {
	c, ok := ctx.(*Ctx)
	if ok {
		return c
	}

	return &Ctx{
		Context: ctx,
		calls:   new(atomic.Int64),
	}
}

func (c *Ctx) IncrCalls() {
	c.calls.Add(1)
}

func (c *Ctx) Calls() int {
	return int(c.calls.Load())
}

// OpenMetricsHandler implements the http.Handler interface
type OpenMetricsHandler struct {
	defaultTimeout time.Duration
	explorer       Explorer
	explorerName   string
}

// NewOpenMetricsHandler create a new OpenMetricsHandler
func NewOpenMetricsHandler(explorerName string, explorer Explorer) *OpenMetricsHandler {
	return &OpenMetricsHandler{
		defaultTimeout: 10 * time.Second,
		explorer:       explorer,
		explorerName:   explorerName,
	}
}

// ServeHTTP implements the http.Handler interface. It collects all metrics from the configured
// collector and return them, formatted in the http response.
func (handler *OpenMetricsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	impacts := make(chan *Impact)
	metrics := make(chan *Metric)
	errs := make(chan error)
	errCount := 0

	traceAttr := slog.Attr{}
	if traceID := r.Header.Get("X-Cloud-Trace-Context"); traceID != "" {
		traceAttr = slog.String("logging.googleapis.com/trace", traceID)
	}

	baseLabels := map[string]string{
		"explorer": handler.explorerName,
	}

	errg, errgctx := errgroup.WithContext(r.Context())
	errgctx, cancel := context.WithTimeout(errgctx, handler.defaultTimeout)
	defer cancel()

	errg.Go(func() error {
		defer close(impacts)
		defer close(errs)

		ctx := WrapCtx(errgctx)
		handler.explorer.CollectImpacts(ctx, impacts, errs)

		metrics <- &Metric{
			Name:   "collect_duration_ms",
			Labels: baseLabels,
			Value:  float64(time.Since(start).Milliseconds()),
		}

		metrics <- &Metric{
			Name:   "error_count",
			Labels: baseLabels,
			Value:  float64(errCount),
		}

		metrics <- &Metric{
			Name:   "api_calls",
			Labels: baseLabels,
			Value:  float64(ctx.Calls()),
		}

		return nil
	})

	errg.Go(func() error {
		defer close(metrics)
		for impact := range impacts {
			impact.Labels = MergeLabels(impact.Labels, baseLabels)
			metrics <- NewEnergyMetric(impact.Energy).SetLabels(impact.Labels)
			metrics <- NewEmissionsMetric(impact.EnergyEmissions).SetLabels(impact.Labels)
			metrics <- NewEmbodiedEmissionsMetric(impact.EmbodiedEmissions).SetLabels(impact.Labels)
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
	metric = metric.SanitizeLabels()

	// sort labels in lexicographical order
	labels := make([]string, 0, len(metric.Labels))
	for labelName, labelValue := range metric.Labels {
		labels = append(labels, fmt.Sprintf(`%s="%s"`, labelName, labelValue))
	}
	slices.SortFunc(labels, strings.Compare)

	_, err := fmt.Fprintf(w, "%s{%s} %0.10f\n", metric.Name, strings.Join(labels, ","), metric.Value)
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

type Impact struct {
	// Labels for impact
	Labels map[string]string
	// Energy in watts
	Energy Energy
	// EnergyEmissions are emissions related to energy in kgCO2eq/day
	EnergyEmissions EmissionsOverTime
	// EmbodiedEmissions are emissions related to the manufacturing
	EmbodiedEmissions EmissionsOverTime
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

func (m *Metric) AddLabel(key, value string) *Metric {
	m.Labels = MergeLabels(
		m.Labels,
		map[string]string{
			key: value,
		},
	)
	return m
}

func (m *Metric) SetLabels(l map[string]string) *Metric {
	m.Labels = l
	return m
}

func (m *Metric) SetValue(v float64) *Metric {
	m.Value = v
	return m
}

func (m *Metric) SanitizeLabels() *Metric {
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

func NewEmbodiedEmissionsMetric(value EmissionsOverTime) *Metric {
	return &Metric{
		Name:  "estimated_embodied_emissions_kgCO2eq_day",
		Value: value.KgCO2eq_day(),
	}
}

func NewEnergyMetric(value Energy) *Metric {
	return &Metric{
		Name:  "estimated_watts",
		Value: float64(value),
	}
}

func NewEmissionsMetric(value EmissionsOverTime) *Metric {
	return &Metric{
		Name:  "estimated_usage_emissions_kgCO2eq_day",
		Value: value.KgCO2eq_day(),
	}
}
