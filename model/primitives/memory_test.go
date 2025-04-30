package primitives

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEstimateMemoryPowerUsage(t *testing.T) {
	assert.Equal(t, 38.0, EstimateMemoryWatts(100))
	assert.Equal(t, 0.76, EstimateMemoryWatts(2))
	assert.Panics(t, func() { EstimateMemoryWatts(-2.9) })
}
