package primitives

// https://cruiseship.cloud/how-much-power-does-a-hard-drive-use/
// Component 			HDD 		SSD
// Power Consumption 	7-12 watts 	1-5 watts
// We take the
func EstimateLocalSSDPowerUsage(diskCount int) (watts float64) {
	return 3.0 * float64(diskCount)
}

func EstimateLocalHDDPowerUsage(diskCount int) (watts float64) {
	return 9.5 * float64(diskCount)
}

// https://cloud.google.com/blog/products/compute/high-durability-persistent-disk
// [...] Each Persistent Disk byte is stored in three or more locations distributed
// across separate fault domains within a given Compute Engine zone.
const rackVolumeReplicationFactor = 3
const rackAverageDiskSizeHypothesis = 2000 // 2 TB
const rackVolumeEnergyOverhead = 1.1       // 10 percent
func EstimateHDDVolume(diskSize float64) (watts float64) {
	return diskSize / rackAverageDiskSizeHypothesis * EstimateLocalHDDPowerUsage(1) * rackVolumeReplicationFactor * rackVolumeEnergyOverhead
}

func EstimateSSDVolume(diskSize float64) (watts float64) {
	return diskSize / rackAverageDiskSizeHypothesis * EstimateLocalSSDPowerUsage(1) * rackVolumeReplicationFactor * rackVolumeEnergyOverhead
}
