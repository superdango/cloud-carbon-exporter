package cloudcarbonfootprint

import (
	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
	"github.com/superdango/cloud-carbon-exporter/internal/must"
	"github.com/superdango/cloud-carbon-exporter/model"
	"github.com/superdango/cloud-carbon-exporter/model/carbon"
)

func NewAWSModel() *model.Model {
	carbonIntensity := carbon.NewAWSCloudCarbonFootprintIntensityMap()

	// data extracted from https://github.com/cloud-carbon-footprint/cloud-carbon-footprint/blob/378d283c9d59fa478e97919705ed321edc9fe28a/packages/aws/src/domain/AwsFootprintEstimationConstants.ts
	primitives := model.Primitives{
		// MISC
		"NETWORKING_COEFFICIENT": {0.001},    // kWh
		"MEMORY_COEFFICIENT":     {0.000392}, // kWh
		"PUE_AVG":                {1.135},

		// DISKs
		"SSDCOEFFICIENT": {1.2},  // watt hours / terabyte
		"HDDCOEFFICIENT": {0.65}, // watt hours / terabyte

		// CPUs Watts min/max per core
		"CPU_VARIABLE":         {0.74, 3.50},
		"CPU_CASCADE_LAKE":     {0.64, 3.97},
		"CPU_SKYLAKE":          {0.65, 4.26},
		"CPU_BROADWELL":        {0.71, 3.69},
		"CPU_HASWELL":          {1.00, 4.74},
		"CPU_COFFEE_LAKE":      {1.14, 5.42},
		"CPU_SANDY_BRIDGE":     {2.17, 8.58},
		"CPU_IVY_BRIDGE":       {3.04, 8.25},
		"CPU_AMD_EPYC_1ST_GEN": {0.82, 2.55},
		"CPU_AMD_EPYC_2ND_GEN": {0.47, 1.69},
		"CPU_AWS_GRAVITON_2":   {0.47, 1.69},

		// GPUs Watts min/max per core
		"GPU_NVIDIA_K520":         {26, 229},
		"GPU_NVIDIA_A10G":         {18, 153},
		"GPU_NVIDIA_T4":           {8, 71},
		"GPU_NVIDIA_TESLA_M60":    {35, 306},
		"GPU_NVIDIA_TESLA_K80":    {35, 306},
		"GPU_NVIDIA_TESLA_V100":   {35, 306},
		"GPU_NVIDIA_TESLA_A100":   {46, 407},
		"GPU_NVIDIA_TESLA_P4":     {9, 76.5},
		"GPU_NVIDIA_TESLA_P100":   {36, 306},
		"GPU_AMD_RADEON_PRO_V520": {26, 229},
	}

	processorPrimitives := model.Primitives{
		"Variable": primitives["CPU_VARIABLE"],

		// https://en.wikipedia.org/wiki/Epyc
		"AMD EPYC 7571":           primitives["CPU_AMD_EPYC_1ST_GEN"],
		"AMD EPYC 7R13 Processor": primitives["CPU_AMD_EPYC_2ND_GEN"], // https://www.phoronix.com/review/amd-epyc-9654-9554-benchmarks/15
		"AMD EPYC 7R32":           primitives["CPU_AMD_EPYC_2ND_GEN"],
		"AMD EPYC 9R14 Processor": primitives["CPU_AMD_EPYC_2ND_GEN"], // https://www.phoronix.com/review/amd-epyc-9654-9554-benchmarks/15

		// https://en.wikipedia.org/wiki/AWS_Graviton
		"AWS Graviton Processor":  primitives["CPU_AWS_GRAVITON_2"],
		"AWS Graviton2 Processor": primitives["CPU_AWS_GRAVITON_2"],
		"AWS Graviton3 Processor": primitives["CPU_AWS_GRAVITON_2"],
		"AWS Graviton4 Processor": primitives["CPU_AWS_GRAVITON_2"],

		// https://en.wikipedia.org/wiki/Haswell_(microarchitecture)
		"High Frequency Intel Xeon E7-8880 v3 (Haswell)": primitives["CPU_HASWELL"],

		// https://en.wikipedia.org/wiki/Skylake_(microarchitecture)
		"Intel Skylake E5 2686 v5":                        primitives["CPU_SKYLAKE"],
		"Intel Xeon 8375C (Ice Lake)":                     primitives["CPU_VARIABLE"],
		"Intel Xeon E5-2650":                              primitives["CPU_HASWELL"],
		"Intel Xeon E5-2666 v3 (Haswell)":                 primitives["CPU_HASWELL"],
		"Intel Xeon E5-2670":                              primitives["CPU_HASWELL"],
		"Intel Xeon E5-2670 (Sandy Bridge)":               primitives["CPU_SANDY_BRIDGE"],
		"Intel Xeon E5-2670 v2 (Ivy Bridge)":              primitives["CPU_IVY_BRIDGE"],
		"Intel Xeon E5-2670 v2 (Ivy Bridge/Sandy Bridge)": primitives["CPU_IVY_BRIDGE"],
		"Intel Xeon E5-2676 v3 (Haswell)":                 primitives["CPU_HASWELL"],
		"Intel Xeon E5-2680 v2 (Ivy Bridge)":              primitives["CPU_IVY_BRIDGE"],
		"Intel Xeon E5-2686 v4 (Broadwell)":               primitives["CPU_BROADWELL"],
		"Intel Xeon Family":                               primitives["CPU_SKYLAKE"],
		"Intel Xeon Platinum 8124M":                       primitives["CPU_SKYLAKE"],
		"Intel Xeon Platinum 8151":                        primitives["CPU_SKYLAKE"],
		"Intel Xeon Platinum 8175":                        primitives["CPU_SKYLAKE"],
		"Intel Xeon Platinum 8175 (Skylake)":              primitives["CPU_SKYLAKE"],
		"Intel Xeon Platinum 8252":                        primitives["CPU_CASCADE_LAKE"],
		"Intel Xeon Platinum 8259 (Cascade Lake)":         primitives["CPU_CASCADE_LAKE"],
		"Intel Xeon Platinum 8259CL":                      primitives["CPU_CASCADE_LAKE"],
		"Intel Xeon Platinum 8275CL (Cascade Lake)":       primitives["CPU_CASCADE_LAKE"],
		"Intel Xeon Platinum 8275L":                       primitives["CPU_CASCADE_LAKE"],
		"Intel Xeon Platinum 8280L (Cascade Lake)":        primitives["CPU_CASCADE_LAKE"],
		"Intel Xeon Scalable (Emerald Rapids)":            primitives["CPU_VARIABLE"],
		"Intel Xeon Scalable (Icelake)":                   primitives["CPU_VARIABLE"],
		"Intel Xeon Scalable (Sapphire Rapids)":           primitives["CPU_VARIABLE"],
		"Intel Xeon Scalable (Skylake)":                   primitives["CPU_SKYLAKE"],
	}

	generateResourceMetrics := func(resource *cloudcarbonexporter.Resource, watts float64) []cloudcarbonexporter.Metric {
		wattsMetric := cloudcarbonexporter.Metric{
			Name:  "estimated_watts",
			Value: watts,
			Labels: cloudcarbonexporter.MergeLabels(resource.Labels, map[string]string{
				"model_version":  "0",
				"cloud_provider": "aws",
				"region":         resource.Region,
				"resource_id":    resource.ID,
				"resource_kind":  resource.Kind,
			}),
		}
		emissions := carbonIntensity.ComputeCO2eq(wattsMetric)
		return []cloudcarbonexporter.Metric{wattsMetric, emissions}
	}

	pue := primitives["PUE_AVG"][0]

	return &model.Model{
		Provider:        "aws",
		CarbonIntensity: carbonIntensity,
		Calculations: map[string]func(r *cloudcarbonexporter.Resource) []cloudcarbonexporter.Metric{
			"ec2/instance": func(r *cloudcarbonexporter.Resource) []cloudcarbonexporter.Metric {
				cpuWatts := r.Source.Float64("ec2_instance_core_count") * processorPrimitives.Linear(
					r.Source.String("ec2_instance_physical_processor"),
					r.Source.Float64("ec2_instance_cpu_usage_percent"),
				)
				memoryWatts := r.Source.Float64("ec2_instance_memory_gb") * primitives["MEMORY_COEFFICIENT"][0]

				return generateResourceMetrics(r, (cpuWatts+memoryWatts)*pue)
			},
		},
	}
}

func LinearCPUWatts(primitives []float64, cpuPercent float64) float64 {
	must.Assert(len(primitives) == 2, "cpu primitives must be an array of two float64 [min, max]")

	minWatt := primitives[0]
	maxWatt := primitives[1]

	return minWatt + ((maxWatt - minWatt) * cpuPercent / 100)
}
