package cloud

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEstimateDisksPowerUsage(t *testing.T) {
	assert.Equal(t, 1.2375, EstimateSSDBlockStorageWatts(1000))
	assert.Equal(t, 1.959375, EstimateHDDBlockStorageWatts(1000))
}
