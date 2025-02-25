package model

import (
	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
)

type AmazonWebServicesModel struct {
	carbonIntensity cloudcarbonexporter.CarbonIntensityMap
	calculations    map[string]func(r *cloudcarbonexporter.Resource) []cloudcarbonexporter.Metric
}

func NewAmazonWebServices() *AmazonWebServicesModel {
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

	return &AmazonWebServicesModel{
		carbonIntensity: carbonIntensity,
		calculations: map[string]func(r *cloudcarbonexporter.Resource) []cloudcarbonexporter.Metric{
			"ec2/instance": func(r *cloudcarbonexporter.Resource) []cloudcarbonexporter.Metric {
				return generateResourceMetrics(r, 10.0)
			},
		},
	}
}

func (aws *AmazonWebServicesModel) Supports(r *cloudcarbonexporter.Resource) bool {
	if r.CloudProvider != "aws" {
		return false
	}

	_, found := aws.calculations[r.Kind]

	return found
}

func (aws *AmazonWebServicesModel) ComputeMetrics(resource *cloudcarbonexporter.Resource) []cloudcarbonexporter.Metric {
	if !aws.Supports(resource) {
		return nil
	}

	return aws.calculations[resource.Kind](resource)
}

func NewAWSCarbonIntensityMap() cloudcarbonexporter.CarbonIntensityMap {
	gcpIntensityMap := NewGCPCarbonIntensityMap()
	awsIntensityMap := make(cloudcarbonexporter.CarbonIntensityMap)

	// awsGcpRegions maps aws regions to gcp regions. AWS emissions are calculated
	// based on GCP data as AWS do not provide those informations.
	var awsGcpRegions = map[string]string{
		"ap-east-1":      "asia-east2",
		"ap-northeast-1": "asia-northeast1",
		"ap-northeast-2": "asia-northeast2",
		"ap-northeast-3": "asia-northeast2",
		"ap-south-1":     "asia-south1",
		"ap-southeast-1": "asia-southeast1",
		"ap-southeast-3": "asia-southeast2",
		"ap-southeast-2": "australia-southeast1",
		"eu-north-1":     "europe-north1",
		"eu-south-1":     "europe-west1",
		"eu-west-1":      "europe-west1",
		"eu-west-2":      "europe-west2",
		"eu-central-1":   "europe-west3",
		"eu-west-3":      "europe-west9",
		"ca-central-1":   "northamerica-northeast1",
		"sa-east-1":      "southamerica-east1 ",
		"us-east-1":      "us-east4",
		"us-east-2":      "us-east4",
		"us-west-1":      "us-west1",
		"us-west-2":      "us-west1",
	}

	for awsRegion, gcpRegion := range awsGcpRegions {
		awsIntensityMap[awsRegion] = gcpIntensityMap.Get(gcpRegion)
	}

	awsIntensityMap["global"] = awsIntensityMap.Average()
	awsIntensityMap["amer"] = awsIntensityMap.Average("us", "ca", "sa")
	awsIntensityMap["apac"] = awsIntensityMap.Average("ap")
	awsIntensityMap["emea"] = awsIntensityMap.Average("eu", "me", "af")

	return awsIntensityMap
}
