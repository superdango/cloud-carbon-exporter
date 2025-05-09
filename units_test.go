package cloudcarbonexporter_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
)

func TestCombineEmissionsOverTime(t *testing.T) {
	e1 := cloudcarbonexporter.EmissionsOverTime{
		Emissions: cloudcarbonexporter.Emissions(1000),
		During:    time.Hour * 24,
	}
	e2 := cloudcarbonexporter.EmissionsOverTime{
		Emissions: cloudcarbonexporter.Emissions(1000),
		During:    time.Hour * 24,
	}
	assert.Equal(t, 2.0, cloudcarbonexporter.CombineEmissionsOverTime(e1, e2).KgCO2eq_day())

	e2 = cloudcarbonexporter.EmissionsOverTime{
		Emissions: cloudcarbonexporter.Emissions(1000),
		During:    time.Hour * 24 * 5,
	}

	assert.Equal(t, 1.2, cloudcarbonexporter.CombineEmissionsOverTime(e1, e2).KgCO2eq_day())

	assert.Equal(t, 0.0, cloudcarbonexporter.CombineEmissionsOverTime(cloudcarbonexporter.ZeroEmissions, cloudcarbonexporter.ZeroEmissions).KgCO2eq_day())

}
