package primitives

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEstimateMemoryPowerUsage(t *testing.T) {
	assert.Equal(t, 38.0, EstimateMemoryPowerUsage(100))
	assert.Equal(t, 0.76, EstimateMemoryPowerUsage(2))
	assert.Panics(t, func() { EstimateMemoryPowerUsage(-2.9) })
}
