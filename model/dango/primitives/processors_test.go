package primitives

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// override embed values for tests
func setupTests() {
	once := new(sync.Once)
	once.Do(func() {
		tdpToPowerRatio = 1
		processors = []Processor{
			{
				Name:    "AMD EPYC 7571",
				Family:  "Naples",
				Tdp:     10,
				Cores:   1,
				Threads: 1,
			},
			{
				Name:    "AMD EPYC 7570",
				Family:  "Naples",
				Tdp:     10,
				Cores:   2,
				Threads: 4,
			},
			{
				Name:    "Intel Xeon",
				Family:  "Saphire",
				Tdp:     1000,
				Cores:   1,
				Threads: 1,
			},
		}
		processorNames = make([]string, len(processors))
		for i, p := range processors {
			processorNames[i] = p.Name
		}
	})
}

func TestCPUPowerUsage(t *testing.T) {
	setupTests()
	watt := FuzzyFindBestProcessor("AMD EPYC 7571").EstimatePowerUsageWithTDP(1, 100)
	assert.Equal(t, 10.0, watt)

	watt = FuzzyFindBestProcessor("EPYC 70").EstimatePowerUsageWithTDP(1, 100)
	assert.Equal(t, 2.5, watt)

	watt = FuzzyFindBestProcessor("Intel").EstimatePowerUsageWithTDP(1, 100)
	assert.Equal(t, 1000.0, watt)

	watt = FuzzyFindBestProcessor("Intel").EstimatePowerUsageWithTDP(1, 0)
	assert.Equal(t, 117.6470588235294, watt)

	watt = FuzzyFindBestProcessor("").EstimatePowerUsageWithTDP(1, 0)
	assert.Equal(t, 117.6470588235294, watt)
}
