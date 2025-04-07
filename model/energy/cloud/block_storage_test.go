package cloud

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEstimateDisksPowerUsage(t *testing.T) {
	assert.Equal(t, 18.0, EstimateSSDBlockStorage(2))
	assert.Equal(t, 57.0, EstimateHDDBlockStorage(2))
}
