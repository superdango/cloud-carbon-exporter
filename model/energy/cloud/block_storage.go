package cloud

import "github.com/superdango/cloud-carbon-exporter/model/energy/primitives"

const rackVolumeReplicationFactor = 3
const rackDisksSize = 2000     // 2 TB
const rackVolumeOverhead = 1.1 // 10 percent
func EstimateHDDBlockStorage(diskSize float64) (watts float64) {
	return diskSize / rackDisksSize * primitives.EstimateLocalHDDPowerUsage(1) * rackVolumeReplicationFactor * rackVolumeOverhead
}

func EstimateSSDBlockStorage(diskSize float64) (watts float64) {
	return diskSize / rackDisksSize * primitives.EstimateLocalSSDPowerUsage(1) * rackVolumeReplicationFactor * rackVolumeOverhead
}
