package cloudcarbonexporter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/superdango/cloud-carbon-exporter/internal/must"
)

type AnyMap map[string]any

func (a AnyMap) Int(k string) int {
	i, ok := a[k].(int)
	must.Assert(ok, "expected a[k] to be int type, got: "+reflect.TypeOf(a[k]).String())
	return i
}

func (a AnyMap) Float64(k string) float64 {
	i, ok := a[k].(float64)
	must.Assert(ok, "expected a[k] to be float64 type, got: "+reflect.TypeOf(a[k]).String())
	return i
}

func (a AnyMap) Bool(k string) bool {
	i, ok := a[k].(bool)
	must.Assert(ok, "expected a[k] to be boolean type, got: "+reflect.TypeOf(a[k]).String())
	return i
}

func (a AnyMap) String(k string) string {
	i, ok := a[k].(string)
	must.Assert(ok, "expected a[k] to be string type, got: "+reflect.TypeOf(a[k]).String())
	return i
}

// Resource is the representation of a Cloud asset potentially drawing energy
type Resource struct {
	// CloudProvider hosting the resource
	CloudProvider string
	// Kind is the type of the resource
	Kind string
	// ID of the resource
	ID string
	// Region can be global, region, zone, etc.
	Region string
	// Labels describing the resource
	Labels map[string]string
	// Source is the raw data collected from the source
	Source AnyMap
}

// Metric olds the name and value of a measurement in addition to its labels.
type Metric struct {
	Name       string
	ResourceID string
	Labels     map[string]string
	Value      float64
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

type Model interface {
	ComputeMetrics(r *Resource) []Metric
	Supports(r *Resource) bool
}

type ExplorerErr struct {
	Err       error
	Operation string
}

func (explorerErr *ExplorerErr) Error() string {
	return fmt.Sprintf("operation failed (op: %s): %s", explorerErr.Operation, explorerErr.Err.Error())
}

func (explorerErr *ExplorerErr) Unwrap() error {
	return explorerErr.Err
}

type Explorer interface {
	Find(ctx context.Context, resources chan *Resource, errors chan error)
	IsReady() bool
	io.Closer
}

type CollectorOptions func(c *Collector)

func WithExplorer(explorer Explorer) CollectorOptions {
	return func(c *Collector) {
		c.explorers = append(c.explorers, explorer)
	}
}

func WithModels(models ...Model) CollectorOptions {
	return func(c *Collector) {
		c.models = models
	}
}

type Collector struct {
	explorers []Explorer
	models    []Model
}

func NewCollector(opts ...CollectorOptions) *Collector {
	collector := &Collector{
		explorers: make([]Explorer, 0),
		models:    make([]Model, 0),
	}

	for _, option := range opts {
		option(collector)
	}

	return collector
}

func (c *Collector) SetOpt(option CollectorOptions) {
	option(c)
}

func (c *Collector) Collect(ctx context.Context, metrics chan Metric) {
	defer close(metrics)

	resources := make(chan *Resource)
	errs := make(chan error)
	errCount := new(atomic.Int32)
	defer func() {
		metrics <- Metric{
			Name: "error_count",
			Labels: map[string]string{
				"action": "collect",
			},
			Value: float64(errCount.Load()),
		}
	}()

	wg := new(sync.WaitGroup)

	wg.Add(1)
	go func() {
		defer wg.Done()
		c.explore(ctx, resources, errs)
	}()

	for {
		select {
		case r, ok := <-resources:
			if !ok {
				wg.Wait()
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				for _, model := range c.models {
					for _, metric := range model.ComputeMetrics(r) {
						metrics <- metric
					}
				}
			}()

		case err, ok := <-errs:
			if !ok {
				wg.Wait()
				return
			}

			if err == nil {
				continue
			}

			errCount.Add(1)

			experr := new(ExplorerErr)
			if errors.As(err, &experr) {
				slog.Warn("resources collection failed", "err", experr, "op", experr.Operation)
				continue
			}

			slog.Warn("failed to explore resources", "err", err)
		}
	}
}

func (c *Collector) Close() error {
	for _, explorer := range c.explorers {
		if err := explorer.Close(); err != nil {
			return err
		}
	}
	return nil
}

func (c *Collector) explore(ctx context.Context, resources chan *Resource, errs chan error) {
	defer close(resources)
	defer close(errs)
	for _, explorer := range c.explorers {
		if !explorer.IsReady() {
			slog.Warn("explorer is not ready", "explorer", reflect.TypeOf(explorer))
			continue
		}
		explorer.Find(ctx, resources, errs)
	}
}

func MergeLabels(labels ...map[string]string) map[string]string {
	result := make(map[string]string)
	for _, l := range labels {
		for k, v := range l {
			if v == "" {
				continue
			}
			result[k] = v
		}
	}
	return result
}
