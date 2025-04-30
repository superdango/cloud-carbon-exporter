package cloud

import (
	"github.com/superdango/cloud-carbon-exporter/model/primitives"
)

func EstimateObjectStorageWatts(bucketSizeGB float64) (watts float64) {
	const jbodDiskSize = 20_000.0 // GB
	const jbodOverhead = 1.4
	const erasureCodingRatio = 1.8
	return bucketSizeGB * erasureCodingRatio / jbodDiskSize * primitives.EstimateLocalHDDPowerUsage(1) * jbodOverhead
}

func EstimateObjectStorageEmbodiedEmissionsKgCO2eq_second(bucketSizeGB float64) (kgCO2eq_second float64) {
	const jbodDiskSize = 20_000.0 // GB
	const erasureCodingRatio = 1.8
	return primitives.EstimateEmbodiedHDDEmissions(bucketSizeGB/jbodDiskSize).KgCO2eq_second() * erasureCodingRatio
}
