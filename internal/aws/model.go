package aws

import (
	"log/slog"
	"os"
	"reflect"

	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
)

// model holds every calculation methods for all resource kind and metrics. If signals
// comes from monitoring api then use "monitoring" model. If resource is directly coming
// from Asset inventory then use "assets" model
type model map[string]func(r *Resource) *cloudcarbonexporter.Metric

// getModelVersion returns the current model versions
func getModelVersion() string {
	return "v0.0.1"
}

func (m model) ComputeResourceEnergyDraw(r *Resource) *cloudcarbonexporter.Metric {
	wattFunc, found := m[r.Arn.FullType()]
	if !found {
		return nil
	}
	return wattFunc(r)
}

func newModel() model {
	return model{
		"ec2/instance": func(r *Resource) *cloudcarbonexporter.Metric {
			instance := mustcast[EC2InstanceRefinedData](r.Source["ec2_instance_data"])
			monitoring := mustcast[EC2InstanceCloudwatchRefinedData](r.Source["ec2_instance_cloudwatch_data"])

			if !instance.Running {
				return nil
			}

			watts := 10*float64(instance.CPU) +
				float64(instance.CPU)*float64(monitoring.CPUUtilizationPercent/100)

			return generateWattMetric(r, watts)
		},
		"ec2/volume": func(r *Resource) *cloudcarbonexporter.Metric {
			return generateWattMetric(r, 1.0)
		},
		"s3": func(r *Resource) *cloudcarbonexporter.Metric {
			monitoring := mustcast[S3BucketCloudwatchRefinedData](r.Source["s3_bucket_cloudwatch_data"])

			return generateWattMetric(r, 0.0000002*monitoring.BucketSizeBytes)
		},

		"ec2/snapshot": func(r *Resource) *cloudcarbonexporter.Metric {
			snapshot := mustcast[EC2SnapshotRefinedData](r.Source["ec2_snapshot_data"])

			return generateWattMetric(r, 0.00002*snapshot.SizeBytes/1000/1000/1000)
		},
	}
}

// isSupportedRessource returns true if model exists for the resource kind
func (m model) isSupportedRessource(resourceKind string) bool {
	if _, ok := m[resourceKind]; ok {
		return true
	}

	return false
}

func mustcast[T any](o any) T {
	if o == nil {
		t := new(T)
		return *t
	}

	casted, ok := o.(T)
	if !ok {
		slog.Error("cast failed, should not happen", "expected", reflect.TypeOf(new(T)), "got", reflect.TypeOf(o))
		os.Exit(1)
	}

	return casted
}

func generateWattMetric(r *Resource, watts float64) *cloudcarbonexporter.Metric {
	return &cloudcarbonexporter.Metric{
		Name:       "estimated_watts",
		ResourceID: r.ID,
		Labels:     generateMetricLabels(r),
		Value:      watts,
	}
}

func generateMetricLabels(r *Resource) map[string]string {
	return cloudcarbonexporter.MergeLabels(r.Labels, map[string]string{
		"cloud_provider": "aws",
		"location":       r.Location,
		"resource_id":    r.Arn.ResourceID,
		"resource_type":  r.Arn.ResourceType,
		"region":         r.Arn.Region,
		"account_id":     r.Arn.AccountID,
		"service":        r.Arn.Service,
		"model_version":  getModelVersion(),
	})
}
