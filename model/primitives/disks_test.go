package primitives

import (
	"testing"

	"github.com/stretchr/testify/assert"
	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
)

func TestEstimateDisksPowerUsage(t *testing.T) {
	assert.Equal(t, cloudcarbonexporter.Energy(3.0), EstimateLocalSSDEnergy(1))
	assert.Equal(t, cloudcarbonexporter.Energy(6.0), EstimateLocalSSDEnergy(2))

	assert.Equal(t, cloudcarbonexporter.Energy(9.5), EstimateLocalHDDEnergy(1))
	assert.Equal(t, cloudcarbonexporter.Energy(19.0), EstimateLocalHDDEnergy(2))

	assert.Equal(t, 0.16, EstimateEmbodiedSSDEmissions(5).KgCO2eq_year())
	assert.Equal(t, 53.7, EstimateEmbodiedHDDEmissions(5).KgCO2eq_year())
}
