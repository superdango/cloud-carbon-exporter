package model

import (
	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
)

type Model_AWS_CloudCarbonFootprint struct {
	carbonIntensity cloudcarbonexporter.CarbonIntensityMap
	calculations    map[string]func(r *cloudcarbonexporter.Resource) []cloudcarbonexporter.Metric
}

func NewAWSCarbonIntensityMap() cloudcarbonexporter.CarbonIntensityMap {
	// Base on the Cloud Carbon Footprint data
	// https://github.com/cloud-carbon-footprint/cloud-carbon-footprint/blob/trunk/packages/aws/src/domain/AwsFootprintEstimationConstants.ts
	// gram / kWh
	awsIntensityMap := cloudcarbonexporter.CarbonIntensityMap{
		"af-south-1":     900.6,
		"ap-east-1":      710.0,
		"ap-south-1":     708.2,
		"ap-northeast-3": 465.8,
		"ap-northeast-2": 415.6,
		"ap-southeast-1": 408.,
		"ap-southeast-2": 760.0,
		"ap-southeast-3": 717.7,
		"ap-northeast-1": 465.8,
		"ca-central-1":   120.0,
		"cn-north-1":     537.4,
		"cn-northwest-1": 537.4,
		"eu-central-1":   311.0,
		"eu-west-1":      278.6,
		"eu-west-2":      225.0,
		"eu-south-1":     213.4,
		"eu-west-3":      51.1,
		"eu-north-1":     8.8,
		"me-south-1":     505.9,
		"me-central-1":   404.1,
		"sa-east-1":      61.7,
		"us-east-1":      379.069,
		"us-east-2":      410.608,
		"us-west-1":      322.167,
		"us-west-2":      322.167,
		"us-gov-east-1":  379.069,
		"us-gov-west-1":  322.167,
	}

	awsIntensityMap["global"] = awsIntensityMap.Average()
	awsIntensityMap["amer"] = awsIntensityMap.Average("us", "ca", "sa")
	awsIntensityMap["apac"] = awsIntensityMap.Average("ap", "cn")
	awsIntensityMap["emea"] = awsIntensityMap.Average("eu", "me", "af")

	return awsIntensityMap
}

// data extracted from https://github.com/cloud-carbon-footprint/cloud-carbon-footprint/blob/378d283c9d59fa478e97919705ed321edc9fe28a/packages/aws/src/domain/AwsFootprintEstimationConstants.ts
var CloudCarbonFootprint_AWSPrimitives = map[string][]float64{
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

var CPUPlatformPrimitives = map[string][]float64{
	// https://en.wikipedia.org/wiki/Epyc
	"AMD EPYC 7571":           CloudCarbonFootprint_AWSPrimitives["CPU_AMD_EPYC_1ST_GEN"],
	"AMD EPYC 7R13 Processor": CloudCarbonFootprint_AWSPrimitives["CPU_AMD_EPYC_2ND_GEN"], // https://www.phoronix.com/review/amd-epyc-9654-9554-benchmarks/15
	"AMD EPYC 7R32":           CloudCarbonFootprint_AWSPrimitives["CPU_AMD_EPYC_2ND_GEN"],
	"AMD EPYC 9R14 Processor": CloudCarbonFootprint_AWSPrimitives["CPU_AMD_EPYC_2ND_GEN"], // https://www.phoronix.com/review/amd-epyc-9654-9554-benchmarks/15

	// https://en.wikipedia.org/wiki/AWS_Graviton
	"AWS Graviton Processor":  CloudCarbonFootprint_AWSPrimitives["CPU_AWS_GRAVITON_2"],
	"AWS Graviton2 Processor": CloudCarbonFootprint_AWSPrimitives["CPU_AWS_GRAVITON_2"],
	"AWS Graviton3 Processor": CloudCarbonFootprint_AWSPrimitives["CPU_AWS_GRAVITON_2"],
	"AWS Graviton4 Processor": CloudCarbonFootprint_AWSPrimitives["CPU_AWS_GRAVITON_2"],

	// https://en.wikipedia.org/wiki/Haswell_(microarchitecture)
	"High Frequency Intel Xeon E7-8880 v3 (Haswell)": CloudCarbonFootprint_AWSPrimitives["CPU_HASWELL"],

	// https://en.wikipedia.org/wiki/Skylake_(microarchitecture)
	"Intel Skylake E5 2686 v5":                        CloudCarbonFootprint_AWSPrimitives["CPU_SKYLAKE"],
	"Intel Xeon 8375C (Ice Lake)":                     CloudCarbonFootprint_AWSPrimitives["CPU_VARIABLE"],
	"Intel Xeon E5-2650":                              CloudCarbonFootprint_AWSPrimitives["CPU_HASWELL"],
	"Intel Xeon E5-2666 v3 (Haswell)":                 CloudCarbonFootprint_AWSPrimitives["CPU_HASWELL"],
	"Intel Xeon E5-2670":                              CloudCarbonFootprint_AWSPrimitives["CPU_HASWELL"],
	"Intel Xeon E5-2670 (Sandy Bridge)":               CloudCarbonFootprint_AWSPrimitives["CPU_SANDY_BRIDGE"],
	"Intel Xeon E5-2670 v2 (Ivy Bridge)":              CloudCarbonFootprint_AWSPrimitives["CPU_IVY_BRIDGE"],
	"Intel Xeon E5-2670 v2 (Ivy Bridge/Sandy Bridge)": CloudCarbonFootprint_AWSPrimitives["CPU_IVY_BRIDGE"],
	"Intel Xeon E5-2676 v3 (Haswell)":                 CloudCarbonFootprint_AWSPrimitives["CPU_HASWELL"],
	"Intel Xeon E5-2680 v2 (Ivy Bridge)":              CloudCarbonFootprint_AWSPrimitives["CPU_IVY_BRIDGE"],
	"Intel Xeon E5-2686 v4 (Broadwell)":               CloudCarbonFootprint_AWSPrimitives["CPU_BROADWELL"],
	"Intel Xeon Family":                               CloudCarbonFootprint_AWSPrimitives["CPU_SKYLAKE"],
	"Intel Xeon Platinum 8124M":                       CloudCarbonFootprint_AWSPrimitives["CPU_SKYLAKE"],
	"Intel Xeon Platinum 8151":                        CloudCarbonFootprint_AWSPrimitives["CPU_SKYLAKE"],
	"Intel Xeon Platinum 8175":                        CloudCarbonFootprint_AWSPrimitives["CPU_SKYLAKE"],
	"Intel Xeon Platinum 8175 (Skylake)":              CloudCarbonFootprint_AWSPrimitives["CPU_SKYLAKE"],
	"Intel Xeon Platinum 8252":                        CloudCarbonFootprint_AWSPrimitives["CPU_CASCADE_LAKE"],
	"Intel Xeon Platinum 8259 (Cascade Lake)":         CloudCarbonFootprint_AWSPrimitives["CPU_CASCADE_LAKE"],
	"Intel Xeon Platinum 8259CL":                      CloudCarbonFootprint_AWSPrimitives["CPU_CASCADE_LAKE"],
	"Intel Xeon Platinum 8275CL (Cascade Lake)":       CloudCarbonFootprint_AWSPrimitives["CPU_CASCADE_LAKE"],
	"Intel Xeon Platinum 8275L":                       CloudCarbonFootprint_AWSPrimitives["CPU_CASCADE_LAKE"],
	"Intel Xeon Platinum 8280L (Cascade Lake)":        CloudCarbonFootprint_AWSPrimitives["CPU_CASCADE_LAKE"],
	"Intel Xeon Scalable (Emerald Rapids)":            CloudCarbonFootprint_AWSPrimitives["CPU_VARIABLE"],
	"Intel Xeon Scalable (Icelake)":                   CloudCarbonFootprint_AWSPrimitives["CPU_VARIABLE"],
	"Intel Xeon Scalable (Sapphire Rapids)":           CloudCarbonFootprint_AWSPrimitives["CPU_VARIABLE"],
	"Intel Xeon Scalable (Skylake)":                   CloudCarbonFootprint_AWSPrimitives["CPU_SKYLAKE"],
	"Variable":                                        CloudCarbonFootprint_AWSPrimitives["CPU_VARIABLE"],
}

func NewModel_AWS_CloudCarbonFootprint() *Model_AWS_CloudCarbonFootprint {
	carbonIntensity := NewAWSCarbonIntensityMap()

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

	min := 0
	//max := 1

	return &Model_AWS_CloudCarbonFootprint{
		carbonIntensity: carbonIntensity,
		calculations: map[string]func(r *cloudcarbonexporter.Resource) []cloudcarbonexporter.Metric{
			"ec2/instance": func(r *cloudcarbonexporter.Resource) []cloudcarbonexporter.Metric {
				primitives := CPUPlatformPrimitives[r.Source.String("ec2_instance_physical_processor")]
				cpuWatts := primitives[min] * r.Source.Float64("ec2_instance_core_count")
				memoryWatts := r.Source.Float64("ec2_instance_memory_gb") * CloudCarbonFootprint_AWSPrimitives["MEMORY_COEFFICIENT"][min]

				return generateResourceMetrics(r, cpuWatts+memoryWatts)
			},
		},
	}
}

func (aws *Model_AWS_CloudCarbonFootprint) Supports(r *cloudcarbonexporter.Resource) bool {
	if r.CloudProvider != "aws" {
		return false
	}

	_, found := aws.calculations[r.Kind]

	return found
}

func (aws *Model_AWS_CloudCarbonFootprint) ComputeMetrics(resource *cloudcarbonexporter.Resource) []cloudcarbonexporter.Metric {
	if !aws.Supports(resource) {
		return nil
	}

	return aws.calculations[resource.Kind](resource)
}
