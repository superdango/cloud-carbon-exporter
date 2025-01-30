package cloudcarbonexporter

import (
	"context"
	"slices"
	"strings"
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
	// Name of the resource
	Name string
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
		if resource.Kind == kind && resource.Name == name {
			return resource, true
		}
	}

	return Resource{}, false
}

// Metric olds the name and value of a measurement in addition to its labels.
type Metric struct {
	Name         string
	ResourceName string
	Labels       map[string]string
	Value        float64
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
