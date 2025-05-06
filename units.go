package cloudcarbonexporter

import (
	"log/slog"
	"time"
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
