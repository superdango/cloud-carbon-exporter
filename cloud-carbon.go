package cloudcarbonexporter

import (
	"context"
	"slices"
	"strings"

	"github.com/superdango/cloud-carbon-exporter/internal/must"
)

// Collector collects metrics and send them directly to metrics channel
type Collector interface {
	Collect(ctx context.Context, metrics chan Metric) error
	Close() error
}

// Resource is the representation of a Cloud asset potentially drawing energy
type Resource struct {
	// Kind is the type of the resource
	Kind string
	// ID of the resource
	ID string
	// Location can be global, region, zone, etc.
	Location string
	// Labels describing the resource
	Labels map[string]string
	// Source is the raw data collected from the source
	Source map[string]any
}

// Resources is a set of resources
type Resources []Resource

// DiscoveredKinds returns the list of kind found in the resources set
func (resources Resources) DiscoveredKinds() []string {
	distinctResources := make([]string, 0)

	for _, resource := range resources {
		distinctResources = append(distinctResources, resource.Kind)
	}

	slices.SortFunc(distinctResources, strings.Compare)
	return slices.Compact(distinctResources)
}

// Find resource by kind and name. Return false is resource is not found.
func (r Resources) Find(kind, name string) (Resource, bool) {
	for _, resource := range r {
		if resource.Kind == kind && resource.ID == name {
			return resource, true
		}
	}

	return Resource{}, false
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
	emissionMetric.Value = intensityMap.Get(wattMetric.Labels["location"]) * wattMetric.Value
	return emissionMetric
}
