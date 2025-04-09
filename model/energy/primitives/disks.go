package primitives

func EstimateLocalSSDPowerUsage(diskCount int) (watts float64) {
	return 3.0 * float64(diskCount)
}

func EstimateLocalHDDPowerUsage(diskCount int) (watts float64) {
	return 9.5 * float64(diskCount)
}
