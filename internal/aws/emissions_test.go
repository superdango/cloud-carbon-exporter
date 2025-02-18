package aws

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLocationCoefficient(t *testing.T) {
	intensityMap := NewCarbonIntensityMap()

	assert.Equal(t, "0.000034", fmt.Sprintf("%.06f", intensityMap.Get("eu-west-1")))
}
