package primitives

import (
	"testing"

	"github.com/stretchr/testify/assert"
	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
)

func TestEstimateMemoryEnergy(t *testing.T) {
	assert.Equal(t, cloudcarbonexporter.Energy(38.0), EstimateMemoryEnergy(100))
	assert.Equal(t, cloudcarbonexporter.Energy(0.76), EstimateMemoryEnergy(2))
	assert.Panics(t, func() { EstimateMemoryEnergy(-2.9) })
}

func TestMemoryEmbodiedEmissions(t *testing.T) {
	assert.InDelta(t, 8.35, EstimateMemoryEmbodiedEmissions(10).KgCO2eq_year(), 0.0000001)
}
