package carbon

import (
	"testing"

	"github.com/stretchr/testify/assert"
	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
)

func TestIntensityMap(t *testing.T) {
	testMap := IntensityMap{
		"global":       2,
		"europe-west1": 1,
		"europe-west2": 2,
		"asia-north3":  3,
		"eu":           1.5,
	}

	testMap["global"] = testMap.Average("europe-west1", "europe-west2", "asia-north3")

	assert.Equal(t, cloudcarbonexporter.Emissions(2.0), testMap.EmissionsPerKWh("global"))
	assert.Equal(t, 1.5, testMap.Average("eu"))
	assert.Equal(t, 1.0, testMap.Average("europe-west1"))
	assert.Equal(t, cloudcarbonexporter.Emissions(1.0), testMap.EmissionsPerKWh("europe-west1"))
}

func TestCO2Compute(t *testing.T) {
	testMap := IntensityMap{
		"global": 1000, // 1 kgCO2eq / kWh
	}

	// if we consume 1000W during 24h we used 24kWh. 1kWh equals 1kgCO2eq therefore
	// this scenario emits 24kgCO2eq
	assert.Equal(t, 24, int(testMap.EnergyEmissions(1000, "global").KgCO2eq_day()))
}
