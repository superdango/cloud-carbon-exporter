package cloud

import (
	"testing"

	"github.com/stretchr/testify/assert"
	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
)

func TestObjectStoragePower(t *testing.T) {
	assert.Equal(t, cloudcarbonexporter.Energy(0.001197), EstimateObjectStorageEnergy(1))
	assert.Equal(t, 96, int(EstimateObjectStorageEmbodiedEmissions(20_000).KgCO2eq_year()*5))
}
