package carbon

// NewAWSCloudCarbonFootprintIntensityMap is based on Cloud Carbon Footprint project estimation for AWS
// https://github.com/cloud-carbon-footprint/cloud-carbon-footprint/blob/trunk/packages/aws/src/domain/AwsFootprintEstimationConstants.ts
func NewAWSCloudCarbonFootprintIntensityMap() IntensityMap {
	awsIntensityMap := IntensityMap{
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
