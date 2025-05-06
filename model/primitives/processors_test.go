package primitives

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
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
			{
				Name:    "Annapurna Labs Graviton2",
				Family:  "Graviton2",
				Tdp:     135,
				Cores:   14,
				Threads: 28,
			},
			{
				Name:    "Annapurna Labs Graviton3",
				Family:  "Graviton3",
				Tdp:     135,
				Cores:   14,
				Threads: 28,
			},
			{
				Name:    "Annapurna Labs Graviton4",
				Family:  "Graviton3",
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
	energy := LookupProcessorByName("AMD EPYC 7571").EstimateCPUEnergy(1, 100)
	assert.Equal(t, cloudcarbonexporter.Energy(10.2), energy)

	energy = LookupProcessorByName("EPYC 70").EstimateCPUEnergy(1, 100)
	assert.Equal(t, cloudcarbonexporter.Energy(2.55), energy)

	energy = LookupProcessorByName("Intel").EstimateCPUEnergy(1, 100)
	assert.Equal(t, cloudcarbonexporter.Energy(1020.0), energy)

	energy = LookupProcessorByName("Intel").EstimateCPUEnergy(1, 0)
	assert.Equal(t, cloudcarbonexporter.Energy(120.0), energy)

	energy = LookupProcessorByName("").EstimateCPUEnergy(1, 0)
	assert.Equal(t, cloudcarbonexporter.Energy(120.0), energy)

	assert.Equal(t, "Intel Xeon E5-2690 V4", LookupProcessorByName("Broadwell").Name)
	assert.Equal(t, "Intel Xeon", LookupProcessorByName("Intel Xeon Family").Name)
	assert.Equal(t, "Annapurna Labs Graviton2", LookupProcessorByName("AWS Graviton2 Processor").Name)
	assert.Equal(t, "Annapurna Labs Graviton3", LookupProcessorByName("AWS Graviton3 Processor").Name)
	assert.Equal(t, "Annapurna Labs Graviton4", LookupProcessorByName("Graviton4").Name)
}

func TestSubMatches(t *testing.T) {
	assert.Equal(t, []string{"foo"}, submatches("foo"))
	assert.Equal(t, []string{"foo", "foo bar", "bar"}, submatches("foo bar"))
	assert.Equal(t, []string{"foo", "foo bar", "foo bar baz", "bar", "baz"}, submatches("foo bar baz"))
}
