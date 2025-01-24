package cloudcarbonexporter

import (
	"context"
	"slices"
	"strings"
)

type Collector interface {
	Collect(ctx context.Context, metrics chan Metric) error
	Close() error
}

type Resource struct {
	Kind     string
	Name     string
	Location string
	Labels   map[string]string
	Metric   *Metric
	Source   map[string]any
}

type Resources []Resource

func (r Resources) DistinctKinds() []string {
	distinctResources := make([]string, 0)

	for _, resource := range r {
		distinctResources = append(distinctResources, resource.Kind)
	}

	slices.SortFunc(distinctResources, strings.Compare)
	return slices.Compact(distinctResources)
}

func (r Resources) Find(kind, name string) (Resource, bool) {
	for _, resource := range r {
		if resource.Kind == kind && resource.Name == name {
			return resource, true
		}
	}

	return Resource{}, false
}

type Metric struct {
	Name         string
	ResourceName string
	Labels       map[string]string
	Value        float64
}

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
