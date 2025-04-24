package machinetypes

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"log/slog"
	"strconv"
	"strings"

	"github.com/superdango/cloud-carbon-exporter/internal/must"
)

//go:embed machine_types.json
var JSONFile []byte

type MachineType struct {
	Name        string  `json:"name"`
	CPUPlatform string  `json:"cpu_platform"`
	VCPU        float64 `json:"vcpu"`
	Memory      float64 `json:"memory"`
	GPU         float64 `json:"gpu"`
	GPUType     string  `json:"gpu_type"`
}

type MachineTypes []MachineType

func MustLoad() MachineTypes {
	machineTypes := new(MachineTypes)
	must.NoError(
		json.NewDecoder(bytes.NewReader(JSONFile)).Decode(machineTypes),
	)
	return *machineTypes
}

func (types MachineTypes) Get(name string) MachineType {
	unknown := MachineType{
		Name: "unknown",
	}

	for _, machineType := range types {
		if machineType.Name == name {
			return machineType
		}
	}

	if strings.HasPrefix(name, "custom-") {
		// format: custom-2-4096
		//                ^ ^
		//                | |
		//                vCPU
		//                  |
		//                  Memory in MB
		splited := strings.Split(name, "-")
		vcpu, err := strconv.Atoi(splited[1])
		if err != nil {
			slog.Warn("bad custom machine type format", "type", name, "err", err.Error())
			return unknown
		}

		memoryMB, err := strconv.Atoi(splited[2])
		if err != nil {
			slog.Warn("bad custom machine type format", "type", name, "err", err.Error())
			return unknown
		}

		return MachineType{
			Name:        "custom",
			VCPU:        float64(vcpu),
			Memory:      float64(memoryMB / 1000),
			CPUPlatform: "Intel Broadwell",
		}
	}

	// https://cloud.google.com/sql/docs/postgres/machine-series-overview#n2_machine_types
	if strings.HasPrefix(name, "perf-optimized-N-") {
		// example: replace perf-optimized-N-2 by n2-highmem-2
		return types.Get("n2-highmem-" + strings.TrimPrefix(name, "perf-optimized-N-"))
	}

	return unknown
}
