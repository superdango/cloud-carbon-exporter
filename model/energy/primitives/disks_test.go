package primitives

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEstimateDisksPowerUsage(t *testing.T) {
	assert.Equal(t, 3.0, EstimateLocalSSDPowerUsage(1))
	assert.Equal(t, 6.0, EstimateLocalSSDPowerUsage(2))
	assert.Equal(t, 18.0, EstimateSSDVolume(2))

	assert.Equal(t, 9.5, EstimateLocalHDDPowerUsage(1))
	assert.Equal(t, 19.0, EstimateLocalHDDPowerUsage(2))
	assert.Equal(t, 57.0, EstimateHDDVolume(2))
}
