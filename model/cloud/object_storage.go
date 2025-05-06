package cloud

import (
	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
	"github.com/superdango/cloud-carbon-exporter/model/primitives"
)

func EstimateObjectStorageEnergy(bucketSizeGB float64) (energy cloudcarbonexporter.Energy) {
	const jbodDiskSize = 20_000.0 // GB
	const jbodOverhead = 1.4
	const erasureCodingRatio = 1.8
	return cloudcarbonexporter.Energy(bucketSizeGB*erasureCodingRatio/jbodDiskSize*jbodOverhead) * primitives.EstimateLocalHDDEnergy(1)
}

func EstimateObjectStorageEmbodiedEmissions(bucketSizeGB float64) cloudcarbonexporter.EmissionsOverTime {
	const jbodDiskSize = 20_000.0 // GB
	const erasureCodingRatio = 1.8
	return primitives.EstimateEmbodiedHDDEmissions(erasureCodingRatio * (bucketSizeGB / jbodDiskSize))
}
