package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
	"github.com/superdango/cloud-carbon-exporter/internal/must"
)

type cpuinfo struct {
}

type EC2InstanceResourceCreator struct {
	awscfg        aws.Config
	defaultRegion string
	cpuinfos      map[string]cpuinfo
}

func NewEC2InstanceResourceCreator(awscfg aws.Config, defaultRegion string) *EC2InstanceResourceCreator {
	return &EC2InstanceResourceCreator{
		awscfg:        awscfg,
		defaultRegion: defaultRegion,
		cpuinfos:      make(map[string]cpuinfo),
	}
}

func (rc *EC2InstanceResourceCreator) Load(ctx context.Context) error {
	ec2api := ec2.NewFromConfig(rc.awscfg, func(o *ec2.Options) {
		o.Region = rc.defaultRegion
	})

	paginator := ec2.NewDescribeInstanceTypesPaginator(ec2api, &ec2.DescribeInstanceTypesInput{})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, instanceType := range page.InstanceTypes {
			must.PrintDebugJSON(map[string]any{
				"instanceType":   instanceType.InstanceType,
				"processor_info": instanceType.ProcessorInfo,
			})
		}
	}
	return nil
}

func (rc *EC2InstanceResourceCreator) CreateResources(ctx context.Context, region string, resources chan *cloudcarbonexporter.Resource) error {
	if region == "global" {
		return nil
	}

	ec2api := ec2.NewFromConfig(rc.awscfg, func(o *ec2.Options) {
		o.Region = region
	})

	paginator := ec2.NewDescribeInstancesPaginator(ec2api, &ec2.DescribeInstancesInput{
		MaxResults: aws.Int32(100),
	})

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list region ec2 instances: %w", err)
		}

		for _, reservation := range output.Reservations {
			for _, instance := range reservation.Instances {
				must.PrintDebugJSON(instance)
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
