package cloud

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestObjectStoragePower(t *testing.T) {
	assert.Equal(t, 0.01197, EstimateObjectStorageWatts(10))
	assert.Equal(t, 96, int(EstimateObjectStorageEmbodiedEmissions(20_000).KgCO2eq_year()*5))
}
