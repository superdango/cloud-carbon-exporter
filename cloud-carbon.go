package cloudcarbonexporter

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/superdango/cloud-carbon-exporter/internal/must"
	"golang.org/x/sync/errgroup"
)

type CollectorOptions func(c *Collector)

func WithExplorer(explorer Explorer) CollectorOptions {
	return func(c *Collector) {
		c.explorers = append(c.explorers, explorer)
	}
}

func WithRefiners(refiners ...Refiner) CollectorOptions {
	return func(c *Collector) {
		c.refiners = append(c.refiners, refiners...)
	}
}

func WithModels(models ...Model) CollectorOptions {
	return func(c *Collector) {
		c.models = models
	}
}

type Collector struct {
	explorers []Explorer
	refiners  []Refiner
	models    []Model
}

func NewCollector(opts ...CollectorOptions) *Collector {
	collector := &Collector{
		explorers: make([]Explorer, 0),
		refiners:  make([]Refiner, 0),
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

func (c *Collector) Collect(ctx context.Context, metrics chan Metric) error {
	defer close(metrics)

	resources := make(chan *Resource)

	errg, errgctx := errgroup.WithContext(ctx)
	errg.SetLimit(5)
	errg.Go(func() error {
		return c.explore(errgctx, resources)
	})

	for {
		select {
		case <-errgctx.Done():
			return errg.Wait()
		case r, ok := <-resources:
			if !ok {
				return errg.Wait()
			}
			errg.Go(func() error {
				if err := c.refine(errgctx, r); err != nil {
					return err
				}

				for _, model := range c.models {
					for _, metric := range model.ComputeMetrics(r) {
						metrics <- metric
					}
				}

				return nil
			})
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

func (c *Collector) explore(ctx context.Context, resources chan *Resource) error {
	defer close(resources)
	for _, explorer := range c.explorers {
		err := explorer.Find(ctx, resources)
		if err != nil {
			return fmt.Errorf("failed to collect resources: %w", err)
		}
	}
	return nil
}

func (c *Collector) refine(ctx context.Context, resource *Resource) error {
	for _, refiner := range c.refiners {
		err := refiner.Refine(ctx, resource)
		if err != nil {
			return fmt.Errorf("failed to refine resource %s: %w", resource.Kind, err)
		}
	}
	return nil
}

type Explorer interface {
	Find(ctx context.Context, resources chan *Resource) error
	io.Closer
}

type Refiner interface {
	Refine(ctx context.Context, resource *Resource) error
	Supports(r *Resource) bool
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
	Source map[string]any
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
	for k, v := range m.Labels {
		copiedLabel[k] = v
	}
	return Metric{
		Name:   m.Name,
		Value:  m.Value,
		Labels: copiedLabel,
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

// CarbonIntensityMap regroups carbon intensity by location
type CarbonIntensityMap map[string]float64

func (intensity CarbonIntensityMap) Average(location ...string) float64 {
	avg := 0.0
	adds := 0.0
	for loc, co2eqsec := range intensity {
		if !hasOnePrefix(loc, location...) {
			continue
		}
		avg = avg + co2eqsec
		adds = adds + 1.0
	}
	avg = avg / adds
	return avg
}

func hasOnePrefix(s string, prefixes ...string) bool {
	if len(prefixes) == 0 {
		return true
	}
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}

	return false
}

func (intensity CarbonIntensityMap) Get(location string) float64 {
	locationsize := 0
	locationIntensity, found := intensity["global"]
	must.Assert(found, "global coefficient not set")

	for l, carbonIntensity := range intensity {
		if strings.HasPrefix(location, l) {
			if len(l) > locationsize {
				locationsize = len(l)
				locationIntensity = carbonIntensity
			}
		}
	}
	return locationIntensity
}

func (intensityMap CarbonIntensityMap) ComputeCO2eq(wattMetric Metric) Metric {
	emissionMetric := wattMetric.Clone()
	emissionMetric.Name = "estimated_g_co2eq_second"
	emissionMetric.Value = intensityMap.Get(wattMetric.Labels["region"]) * wattMetric.Value
	return emissionMetric
}

// EnergyModel holds every calculation methods for all supported resources.
type EnergyModel map[string]func(r *Resource) *Metric

func (m EnergyModel) EstimateWatts(r *Resource) *Metric {
	if formula, found := m[r.Kind]; found {
		return formula(r)
	}

	return nil
}

type Model interface {
	ComputeMetrics(r *Resource) []Metric
	Supports(r *Resource) bool
}
