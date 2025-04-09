package cloud

import "github.com/superdango/cloud-carbon-exporter/model/energy/primitives"

const rackVolumeReplicationFactor = 3
const ssdRackDisksSize = 8000  // 8  TB
const hddRackDiskSize = 16000  // 16 TB
const rackVolumeOverhead = 1.1 // 10 percent
func EstimateHDDBlockStorage(diskSize float64) (watts float64) {
	return diskSize / hddRackDiskSize * primitives.EstimateLocalHDDPowerUsage(1) * rackVolumeReplicationFactor * rackVolumeOverhead
}

func EstimateSSDBlockStorage(diskSize float64) (watts float64) {
	return diskSize / ssdRackDisksSize * primitives.EstimateLocalSSDPowerUsage(1) * rackVolumeReplicationFactor * rackVolumeOverhead
}
