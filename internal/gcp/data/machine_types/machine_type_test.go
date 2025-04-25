package machinetypes

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCustomMachineType(t *testing.T) {
	mt := MustLoad()

	assert.Equal(t, MachineType{
		Name:        "custom",
		VCPU:        2,
		Memory:      2,
		CPUPlatform: "Intel Broadwell",
	}, mt.Get("custom-2-2048"))

	assert.Equal(t, MachineType{
		Name:        "n2-highmem-2",
		VCPU:        2,
		Memory:      16,
		CPUPlatform: "Intel Cascade Lake, Intel Ice Lake",
	}, mt.Get("perf-optimized-N-2"))

	assert.Equal(t, MachineType{
		Name:        "custom",
		VCPU:        2,
		Memory:      1,
		CPUPlatform: "AMD Milan",
	}, mt.Get("n2d-custom-2-1024"))
}
