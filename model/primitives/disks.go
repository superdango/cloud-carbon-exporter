package primitives

func EstimateLocalSSDPowerUsage(diskCount int) (watts float64) {
	return 3.0 * float64(diskCount)
}

func EstimateLocalHDDPowerUsage(diskCount int) (watts float64) {
	return 9.5 * float64(diskCount)
}

// HDDEmissions embodied emissions
// Source: Makara Enterprise HDDEmissions Product Life Cycle Assessment (LCA) Summary
// https://www.seagate.com/files/www-content/global-citizenship/en-us/docs/seagate-makara-enterprise-hdd-lca-summary-2016-07-29.pdf
func EstimateEmbodiedHDDEmissions(count float64) EmbodiedEmissions {
	return EmbodiedEmissions{
		manufacturingEmissions: kgCO2eq(percent(15) * 358 * float64(count)),
		lifetime:               5 * YEAR,
	}
}

// EstimateEmbodiedSSDEmissions embodied emissions
// https://hotcarbon.org/assets/2022/pdf/hotcarbon22-tannu.pdf#cite.ICT1
// Page 3: Our evaluations show that SSDs have SEF equal to 0.16 Kg-CO2e/GB on average
func EstimateEmbodiedSSDEmissions(sizeGB float64) EmbodiedEmissions {
	return EmbodiedEmissions{
		manufacturingEmissions: kgCO2eq(0.16 * sizeGB),
		lifetime:               5 * YEAR,
	}
}
