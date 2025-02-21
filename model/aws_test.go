package model

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAWSLocationCoefficient(t *testing.T) {
	intensityMap := NewAWSCarbonIntensityMap()

	assert.Equal(t, "0.000034", fmt.Sprintf("%.06f", intensityMap.Get("eu-west-1")))
}
