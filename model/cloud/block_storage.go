package cloud

import (
	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
	"github.com/superdango/cloud-carbon-exporter/model/primitives"
)

const rackVolumeReplicationFactor = 3
const ssdRackDisksSize = 8000  // 8  TB
const hddRackDiskSize = 16000  // 16 TB
const rackVolumeOverhead = 1.1 // 10 percent
func EstimateHDDBlockStorageEnergy(diskSize float64) (energy cloudcarbonexporter.Energy) {
	return cloudcarbonexporter.Energy(diskSize/hddRackDiskSize*rackVolumeReplicationFactor*rackVolumeOverhead) * primitives.EstimateLocalHDDEnergy(1)
}

func EstimateHDDBlockStorageEmbodiedEmissions(diskSize float64) (emissions cloudcarbonexporter.EmissionsOverTime) {
	return primitives.EstimateEmbodiedHDDEmissions((diskSize * rackVolumeReplicationFactor) / hddRackDiskSize)
}

func EstimateSSDBlockStorageEnergy(diskSize float64) (energy cloudcarbonexporter.Energy) {
	return cloudcarbonexporter.Energy(diskSize/ssdRackDisksSize*rackVolumeReplicationFactor*rackVolumeOverhead) * primitives.EstimateLocalSSDEnergy(1)
}

func EstimateSSDBlockStorageEmbodiedEmissions(diskSize float64) (emissions cloudcarbonexporter.EmissionsOverTime) {
	return primitives.EstimateEmbodiedSSDEmissions(diskSize * rackVolumeReplicationFactor)
}
