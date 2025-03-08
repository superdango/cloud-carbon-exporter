package model

import (
	"strings"

	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
	"github.com/superdango/cloud-carbon-exporter/internal/must"
)

type Model struct {
	Provider        string
	CarbonIntensity CarbonIntensityMap
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

func (intensityMap CarbonIntensityMap) ComputeCO2eq(wattMetric cloudcarbonexporter.Metric) cloudcarbonexporter.Metric {
	emissionMetric := wattMetric.Clone()
	emissionMetric.Name = "estimated_g_co2eq_second"
	gramPerKWh := intensityMap.Get(wattMetric.Labels["region"]) / 1000 / 60 / 60
	emissionMetric.Value = wattMetric.Value * gramPerKWh
	return emissionMetric
}
