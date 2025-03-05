package model

import (
	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
)

type Model struct {
	Provider        string
	CarbonIntensity cloudcarbonexporter.CarbonIntensityMap
	Calculations    map[string]func(r *cloudcarbonexporter.Resource) []cloudcarbonexporter.Metric
}

func (model *Model) Supports(r *cloudcarbonexporter.Resource) bool {
	if r.CloudProvider != model.Provider {
		return false
	}

	_, found := model.Calculations[r.Kind]

	return found
}

func (model *Model) ComputeMetrics(r *cloudcarbonexporter.Resource) []cloudcarbonexporter.Metric {
	if !model.Supports(r) {
		return nil
	}

	for kind, calculation := range model.Calculations {
		if kind == r.Kind {
			return calculation(r)
		}
	}

	return nil
}

type Primitives map[string][]float64

func (p Primitives) Linear(primitive string, percent float64) float64 {
	primitives, found := p[primitive]
	if !found {
		primitives = p["DEFAULT"]
	}

	if len(primitives) == 1 {
		return primitives[0]
	}

	min := primitives[0]
	max := primitives[1]

	return min + ((max - min) * percent / 100)
}
