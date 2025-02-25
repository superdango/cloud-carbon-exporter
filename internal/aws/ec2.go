package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
)

func (e *Explorer) listRegionEC2Instances(ctx context.Context, region string, resources chan *cloudcarbonexporter.Resource) error {
	if region == "global" {
		return nil
	}

	ec2api := ec2.NewFromConfig(e.awscfg, func(o *ec2.Options) {
		o.Region = region
	})

	paginator := ec2.NewDescribeInstancesPaginator(ec2api, &ec2.DescribeInstancesInput{
		MaxResults: aws.Int32(100),
	})

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list region instances: %w", err)
		}

		for _, reservation := range output.Reservations {
			for _, instance := range reservation.Instances {
				resources <- &cloudcarbonexporter.Resource{
					CloudProvider: "aws",
					Kind:          "ec2/instance",
					ID:            *instance.InstanceId,
					Region:        region,
					Labels:        parseEC2Tags(instance.Tags),
					Source: map[string]any{
						"ec2_instance_core_count": int(*instance.CpuOptions.CoreCount),
						"ec2_instance_is_running": instance.State.Name == types.InstanceStateNameRunning,
					},
				}
			}
		}
	}

	return nil
}

func parseEC2Tags(tags []types.Tag) map[string]string {
	m := make(map[string]string, len(tags))

	for _, t := range tags {
		m[fmt.Sprintf("tag_%s", *t.Key)] = *t.Value
	}

	return m
}
