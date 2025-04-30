package cloud

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestObjectStoragePower(t *testing.T) {
	assert.Equal(t, 0.01197, EstimateObjectStorageWatts(10))
	assert.Equal(t, 96, int(EstimateObjectStorageEmbodiedEmissionsKgCO2eq_second(20_000)*60*60*24*365*5))
}
