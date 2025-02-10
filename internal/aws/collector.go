package aws

import (
	"cloudcarbonexporter"
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/resourceexplorer2"
)

type Collector struct {
	explorer *resourceexplorer2.Client
}

func NewCollector(ctx context.Context, cfg aws.Config) *Collector {
	explorer := resourceexplorer2.NewFromConfig(cfg)
	return &Collector{
		explorer: explorer,
	}
}

func (c *Collector) Collect(ctx context.Context, metrics chan cloudcarbonexporter.Metric) error {
	defer close(metrics)

	list, err := c.explorer.ListResources(ctx, &resourceexplorer2.ListResourcesInput{})
	if err != nil {
		return fmt.Errorf("failed to list aws resources: %w", err)
	}

	for _, r := range list.Resources {
		if *r.Service == "s3" {
			fmt.Println("type", *r.ResourceType, "props: ", r.Properties)
		}
	}

	return nil
}

func (c *Collector) Close() error {
	return nil
}
