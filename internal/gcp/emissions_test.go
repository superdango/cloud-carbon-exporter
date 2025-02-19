package gcp

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEmissions(t *testing.T) {
	intensity := NewCarbonIntensityMap()

	assert.Len(t, intensity, 44)
	assert.Equal(t, "0.000094", fmt.Sprintf("%.06f", intensity.Average()))
	assert.Equal(t, "0.000062", fmt.Sprintf("%.06f", intensity.Average("eu")))
	assert.Equal(t, "0.000078", fmt.Sprintf("%.06f", intensity.Average("eu", "us")))
	assert.Equal(t, "0.000074", fmt.Sprintf("%.06f", intensity.Average("amer")))
}
