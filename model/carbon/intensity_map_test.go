package carbon

import (
	"testing"

	"github.com/stretchr/testify/assert"
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

	assert.Equal(t, 2.0, testMap.Get("global"))
	assert.Equal(t, 1.5, testMap.Average("eu"))
	assert.Equal(t, 1.0, testMap.Average("europe-west1"))
	assert.Equal(t, 1.0, testMap.Get("europe-west1"))
}
