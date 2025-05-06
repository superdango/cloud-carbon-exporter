package cloud

import (
	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
	"github.com/superdango/cloud-carbon-exporter/model/primitives"
)

func EstimateObjectStorageWatts(bucketSizeGB float64) (watts cloudcarbonexporter.Energy) {
	const jbodDiskSize = 20_000.0 // GB
	const jbodOverhead = 1.4
	const erasureCodingRatio = 1.8
	return cloudcarbonexporter.Energy(bucketSizeGB * erasureCodingRatio / jbodDiskSize * primitives.EstimateLocalHDDPowerUsage(1) * jbodOverhead)
}

func EstimateObjectStorageEmbodiedEmissions(bucketSizeGB float64) *primitives.EmbodiedEmissions {
	const jbodDiskSize = 20_000.0 // GB
	const erasureCodingRatio = 1.8
	return primitives.EstimateEmbodiedHDDEmissions(erasureCodingRatio * (bucketSizeGB / jbodDiskSize))
}
