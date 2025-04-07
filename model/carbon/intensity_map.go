package carbon

import (
	"strings"

	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
	"github.com/superdango/cloud-carbon-exporter/internal/must"
)

// IntensityMap regroups carbon intensity by location
type IntensityMap map[string]float64

func (intensity IntensityMap) Average(location ...string) float64 {
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

func (intensity IntensityMap) Get(location string) float64 {
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

// ComputeCO2eq takes an energy metric as input and return its carbon emission equivalent using
// the source region label.
func (intensityMap IntensityMap) ComputeCO2eq(wattMetric *cloudcarbonexporter.Metric) *cloudcarbonexporter.Metric {
	if _, found := wattMetric.Labels["region"]; !found {
		return nil
	}
	emissionMetric := wattMetric.Clone()
	emissionMetric.Name = "estimated_g_co2eq_second"
	gramPerKWh := intensityMap.Get(wattMetric.Labels["region"]) / 1000 / 60 / 60
	emissionMetric.Value = wattMetric.Value * gramPerKWh
	return &emissionMetric
}
