package primitives

import (
	"log/slog"
	"time"
)

const YEAR = 365 * 24 * time.Hour

func percent(n float64) float64 {
	return n / 100.0
}

type kgCO2eq float64
type EmbodiedEmissions struct {
	lifetime               time.Duration
	manufacturingEmissions kgCO2eq
}

func (k EmbodiedEmissions) KgCO2eq_second() float64 {
	if k.lifetime == 0 {
		slog.Warn("embodied emission lifetime is not set, should not happen. Please consider raising a bug.")
		k.lifetime = YEAR
	}
	return float64(k.manufacturingEmissions) / k.lifetime.Seconds()
}

func (k EmbodiedEmissions) KgCO2eq_year() float64 {
	return k.KgCO2eq_second() * 60 * 60 * 24 * 365
}

