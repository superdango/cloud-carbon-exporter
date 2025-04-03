package cloudcarbonexporter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"reflect"
	"sync/atomic"
)

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
	CollectMetrics(ctx context.Context, metrics chan *Metric, errors chan error)
	IsReady() bool
	io.Closer
}

type CollectorOptions func(c *Collector)

func WithExplorer(explorer Explorer) CollectorOptions {
	return func(c *Collector) {
		c.explorers = append(c.explorers, explorer)
	}
}

type Collector struct {
	explorers []Explorer
}

func NewCollector(opts ...CollectorOptions) *Collector {
	collector := &Collector{
		explorers: make([]Explorer, 0),
	}

	for _, option := range opts {
		option(collector)
	}

	return collector
}

func (c *Collector) SetOpt(option CollectorOptions) {
	option(c)
}

func (c *Collector) CollectMetrics(ctx context.Context, metrics chan *Metric) {
	errs := make(chan error)
	errCount := new(atomic.Int32)

	go func() {
		defer close(errs)
		c.explore(ctx, metrics, errs)
	}()

	for err := range errs {
		if err == nil {
			continue
		}

		errCount.Add(1)

		experr := new(ExplorerErr)
		if errors.As(err, &experr) {
			slog.Warn("metrics collection failed", "err", experr, "op", experr.Operation)
			continue
		}
	}

	metrics <- &Metric{
		Name: "error_count",
		Labels: map[string]string{
			"action": "collect",
		},
		Value: float64(errCount.Load()),
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

func (c *Collector) explore(ctx context.Context, metrics chan *Metric, errs chan error) {
	for _, explorer := range c.explorers {
		if !explorer.IsReady() {
			slog.Warn("explorer is not ready", "explorer", reflect.TypeOf(explorer))
			continue
		}
		explorer.CollectMetrics(ctx, metrics, errs)
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
