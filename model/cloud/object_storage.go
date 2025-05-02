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

func EstimateObjectStorageEmbodiedEmissions(bucketSizeGB float64) *primitives.EmbodiedEmissions {
	const jbodDiskSize = 20_000.0 // GB
	const erasureCodingRatio = 1.8
	return primitives.EstimateEmbodiedHDDEmissions(erasureCodingRatio * (bucketSizeGB / jbodDiskSize))
}
