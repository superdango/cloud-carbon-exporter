package cloud

import "github.com/superdango/cloud-carbon-exporter/model/primitives"

const rackVolumeReplicationFactor = 3
const ssdRackDisksSize = 8000  // 8  TB
const hddRackDiskSize = 16000  // 16 TB
const rackVolumeOverhead = 1.1 // 10 percent
func EstimateHDDBlockStorageWatts(diskSize float64) (watts float64) {
	return diskSize / hddRackDiskSize * primitives.EstimateLocalHDDPowerUsage(1) * rackVolumeReplicationFactor * rackVolumeOverhead
}

func EstimateHDDBlockStorageEmbodiedEmissionsKgCO2eq_second(diskSize float64) (kgCO2eq_second float64) {
	return primitives.EstimateEmbodiedHDDEmissions(diskSize/hddRackDiskSize).KgCO2eq_second() * rackVolumeReplicationFactor
}

func EstimateSSDBlockStorageWatts(diskSize float64) (watts float64) {
	return diskSize / ssdRackDisksSize * primitives.EstimateLocalSSDPowerUsage(1) * rackVolumeReplicationFactor * rackVolumeOverhead
}

func EstimateSSDBlockStorageEmbodiedEmissionsKgCO2eq_second(diskSize float64) (kgCO2eq_second float64) {
	return primitives.EstimateEmbodiedSSDEmissions(diskSize).KgCO2eq_second() * rackVolumeReplicationFactor
}
