package cloud

import (
	"testing"

	"github.com/stretchr/testify/assert"
	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
)

func TestEstimateDisksPowerUsage(t *testing.T) {
	assert.Equal(t, cloudcarbonexporter.Energy(1.2375), EstimateSSDBlockStorageEnergy(1000))
	assert.Equal(t, cloudcarbonexporter.Energy(1.959375), EstimateHDDBlockStorageEnergy(1000))
}
