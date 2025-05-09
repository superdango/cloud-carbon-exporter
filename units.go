package cloudcarbonexporter

import (
	"log/slog"
	"time"

	"github.com/superdango/cloud-carbon-exporter/internal/must"
)

// Energy in watt
type Energy float64

// Emissions in gCO2eq
type Emissions float64

func (e Emissions) KgCO2eq() float64 {
	return float64(e) / 1000
}

func (e Emissions) TCO2eq() float64 {
	return e.KgCO2eq() / 1000
}

type EmissionsOverTime struct {
	During    time.Duration
	Emissions Emissions
}

var ZeroEmissions = EmissionsOverTime{During: time.Hour, Emissions: 0}

func CombineEmissionsOverTime(eots ...EmissionsOverTime) EmissionsOverTime {
	must.Assert(len(eots) > 1, "must combine at least two emissions over time")
	base := eots[0]
	for i := 1; i < len(eots); i++ {
		factor := base.During.Seconds() / eots[i].During.Seconds()
		toAdd := eots[i].Emissions * Emissions(factor)
		base.Emissions += toAdd
	}
	return base
}

func (k EmissionsOverTime) KgCO2eq_second() float64 {
	if k.During == 0 {
		slog.Warn("embodied emission lifetime is not set, should not happen. Please consider raising a bug.")
		k.During = 5 * (time.Hour * 24 * 365) // 5 years
	}
	return k.Emissions.KgCO2eq() / k.During.Seconds()
}

func (k EmissionsOverTime) KgCO2eq_day() float64 {
	return k.KgCO2eq_second() * 60 * 60 * 24
}

func (k EmissionsOverTime) KgCO2eq_year() float64 {
	return k.KgCO2eq_day() * 365
}
