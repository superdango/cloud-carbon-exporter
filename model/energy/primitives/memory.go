package primitives

import "github.com/superdango/cloud-carbon-exporter/internal/must"

// In their example, we can see a power consumption of ~0,41W/GB.
// In its official documentation, memory manufacturer Crucial [9]
// says that: “As a rule of thumb, however, you want to allocate
// around 3 watts of power for every 8GB of DDR3 or DDR4 memory”.
// Which equals to ~0,38W/GB.
// https://medium.com/teads-engineering/estimating-aws-ec2-instances-power-consumption-c9745e347959
func EstimateMemoryPowerUsage(gigabytes float64) (watts float64) {
	must.Assert(gigabytes > 0, "memory must be greater than 0")
	return 0.38 * gigabytes
}
