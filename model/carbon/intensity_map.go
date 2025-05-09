package carbon

import (
	"log/slog"
	"strings"
	"time"

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

func (intensity IntensityMap) EmissionsPerKWh(location string) cloudcarbonexporter.Emissions {
	location = strings.ToLower(location)
	locationsize := 0
	locationIntensity := 0.0

	for l, carbonIntensity := range intensity {
		if strings.HasPrefix(location, l) {
			if len(l) > locationsize {
				locationsize = len(l)
				locationIntensity = carbonIntensity
			}
		}
	}
	if locationIntensity == 0.0 {
		slog.Debug("location co2 intensity not found, assuming global intensity", "location", location)
		var found bool
		locationIntensity, found = intensity["global"]
		must.Assert(found, "global coefficient not set")
	}

	return cloudcarbonexporter.Emissions(locationIntensity)
}

// EnergyEmissions takes an energy value as input and return its carbon emission equivalent using
// the source location label.
func (intensityMap IntensityMap) EnergyEmissions(energy cloudcarbonexporter.Energy, location string) (emissions cloudcarbonexporter.EmissionsOverTime) {
	kW := float64(energy / 1000)
	return cloudcarbonexporter.EmissionsOverTime{
		Emissions: cloudcarbonexporter.Emissions(kW) * intensityMap.EmissionsPerKWh(location),
		During:    1 * time.Hour,
	}
}
