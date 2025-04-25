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

	cpuPlatforms := map[string]string{
		"c4":  "Intel Emerald Rapids",
		"c4a": "Google Axion",
		"c4d": "AMD Turin",
		"n4":  "Intel Emerald Rapids",
		"c3":  "Intel Sapphire Rapids",
		"c3d": "AMD Genoa",
		"e2":  "Intel Broadwell",
		"n2":  "Intel Cascade Lake",
		"n2d": "AMD Milan",
		"t2a": "Ampere Altra",
		"t2d": "AMD Milan",
		"n1":  "Intel Haswell",
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

	if strings.Contains(name, "-custom-") {
		// format: n1-custom-2-4096
		//         ^      ^ ^
		//         |      | |
		//      platform  vCPU
		//                  |
		//                  Memory in MB
		splited := strings.Split(name, "-")

		platform := "Intel Broadwell"
		if p, found := cpuPlatforms[splited[0]]; found {
			platform = p
		}

		vcpu, err := strconv.Atoi(splited[2])
		if err != nil {
			slog.Warn("bad custom machine type format", "type", name, "err", err.Error())
			return unknown
		}

		memoryMB, err := strconv.Atoi(splited[3])
		if err != nil {
			slog.Warn("bad custom machine type format", "type", name, "err", err.Error())
			return unknown
		}

		return MachineType{
			Name:        "custom",
			VCPU:        float64(vcpu),
			Memory:      float64(memoryMB / 1000),
			CPUPlatform: platform,
		}
	}

	return unknown
}
