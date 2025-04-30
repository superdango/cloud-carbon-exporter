package primitives

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEstimateDisksPowerUsage(t *testing.T) {
	assert.Equal(t, 3.0, EstimateLocalSSDPowerUsage(1))
	assert.Equal(t, 6.0, EstimateLocalSSDPowerUsage(2))

	assert.Equal(t, 9.5, EstimateLocalHDDPowerUsage(1))
	assert.Equal(t, 19.0, EstimateLocalHDDPowerUsage(2))

	assert.Equal(t, 0.16, EstimateEmbodiedSSDEmissions(5).KgCO2eq_year())
	assert.Equal(t, 53.7, EstimateEmbodiedHDDEmissions(5).KgCO2eq_year())
}
