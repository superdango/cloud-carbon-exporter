package primitives

import (
	"time"

	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
)

const YEAR = 365 * 24 * time.Hour

func EstimateLocalSSDEnergy(diskCount int) (energy cloudcarbonexporter.Energy) {
	return cloudcarbonexporter.Energy(3.0 * float64(diskCount))
}

func EstimateLocalHDDEnergy(diskCount int) (energy cloudcarbonexporter.Energy) {
	return cloudcarbonexporter.Energy(9.5 * float64(diskCount))
}

// HDDEmissions embodied emissions
// Source: Makara Enterprise HDDEmissions Product Life Cycle Assessment (LCA) Summary
// https://www.seagate.com/files/www-content/global-citizenship/en-us/docs/seagate-makara-enterprise-hdd-lca-summary-2016-07-29.pdf
func EstimateEmbodiedHDDEmissions(count float64) cloudcarbonexporter.EmissionsOverTime {
	kgCO2eq := cloudcarbonexporter.Emissions(percent(15) * 358 * float64(count))
	gCO2eq := kgCO2eq * 1000
	return cloudcarbonexporter.EmissionsOverTime{
		Emissions: gCO2eq,
		During:    4 * YEAR,
	}
}

// EstimateEmbodiedSSDEmissions embodied emissions
// https://hotcarbon.org/assets/2022/pdf/hotcarbon22-tannu.pdf#cite.ICT1
// Page 3: Our evaluations show that SSDs have SEF equal to 0.16 Kg-CO2e/GB on average
func EstimateEmbodiedSSDEmissions(sizeGB float64) cloudcarbonexporter.EmissionsOverTime {
	kgCO2eq := cloudcarbonexporter.Emissions(0.16 * sizeGB)
	gCO2eq := kgCO2eq * 1000
	return cloudcarbonexporter.EmissionsOverTime{
		Emissions: gCO2eq,
		During:    4 * YEAR,
	}
}

func percent(n float64) float64 {
	return n / 100.0
}
