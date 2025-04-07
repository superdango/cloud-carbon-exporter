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
			{
				Name:    "Intel Xeon E5-2690 V4",
				Family:  "Intel Broadwell",
				Tdp:     135,
				Cores:   14,
				Threads: 28,
			},
		}
		processorFullNames = make([]string, len(processors))
		for i, p := range processors {
			processorFullNames[i] = p.Name + " " + p.Family
		}
	})
}

func TestCPUPowerUsage(t *testing.T) {
	setupTests()
	watt := LookupProcessorByName("AMD EPYC 7571").EstimatePowerUsageWithTDP(1, 100)
	assert.Equal(t, 10.2, watt)

	watt = LookupProcessorByName("EPYC 70").EstimatePowerUsageWithTDP(1, 100)
	assert.Equal(t, 2.55, watt)

	watt = LookupProcessorByName("Intel").EstimatePowerUsageWithTDP(1, 100)
	assert.Equal(t, 1020.0, watt)

	watt = LookupProcessorByName("Intel").EstimatePowerUsageWithTDP(1, 0)
	assert.Equal(t, 120.0, watt)

	watt = LookupProcessorByName("").EstimatePowerUsageWithTDP(1, 0)
	assert.Equal(t, 120.0, watt)

	assert.Equal(t, "Intel Xeon E5-2690 V4", LookupProcessorByName("Broadwell").Name)
	assert.Equal(t, "Intel Xeon", LookupProcessorByName("Intel Xeon Family").Name)
}

func TestSubMatches(t *testing.T) {
	assert.Equal(t, []string{"foo"}, submatches("foo"))
	assert.Equal(t, []string{"foo", "foo bar"}, submatches("foo bar"))
	assert.Equal(t, []string{"foo", "foo bar", "foo bar baz"}, submatches("foo bar baz"))
}
