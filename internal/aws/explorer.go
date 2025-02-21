package aws

import (
	"context"
	"fmt"
	"log/slog"

	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	resourcetypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
)

type config struct {
	awscfg              aws.Config
	regions             []string
	supportResourceFunc func(r *cloudcarbonexporter.Resource) bool
}

type Explorer struct {
	config *config
}

type Option func(c *config)

func Config(cfg aws.Config) Option {
	return func(c *config) {
		c.awscfg = cfg
	}
}

func Regions(regions ...string) Option {
	return func(c *config) {
		c.regions = regions
	}
}

func SupportsFunc(support func(r *cloudcarbonexporter.Resource) bool) Option {
	return func(c *config) {
		c.supportResourceFunc = support
	}
}

// Refiner adds data on the fly to discovered resource like monitoring or apis
type Refiner interface {
	Refine(ctx context.Context, r *Resource) error
	Supports(r *Resource) bool
}

func NewExplorer(ctx context.Context, opts ...Option) *Explorer {
	c := &config{
		regions: []string{"eu-west-1", "eu-west-3", "us-west-1"},
	}

	for _, opt := range opts {
		opt(c)
	}

	return &Explorer{
		config: c,
	}
}

type Resource struct {
	cloudcarbonexporter.Resource
	Arn *ARN
}

// discoverResources list all supported resources that have been tagged in configured regions
func (explorer *Explorer) Find(ctx context.Context, resources chan *cloudcarbonexporter.Resource) error {
	defer close(resources)

	for _, region := range explorer.config.regions {
		api := resourcegroupstaggingapi.NewFromConfig(explorer.config.awscfg, func(o *resourcegroupstaggingapi.Options) {
			o.Region = region
		})

		paginator := resourcegroupstaggingapi.NewGetResourcesPaginator(api, &resourcegroupstaggingapi.GetResourcesInput{
			ResourcesPerPage: aws.Int32(100),
		})

		for paginator.HasMorePages() {
			page, err := paginator.NextPage(ctx)
			if err != nil {
				return fmt.Errorf("failed to get resources: %w", err)
			}

			for _, resourcetag := range page.ResourceTagMappingList {
				arn, err := NewARN(*resourcetag.ResourceARN)
				if err != nil {
					return fmt.Errorf("failed to parse resource arn: %w", err)
				}

				r := &cloudcarbonexporter.Resource{
					CloudProvider: "aws",
					Kind:          arn.FullType(),
					ID:            arn.ResourceID,
					Region:        arn.Region,
					Labels:        parseTags(resourcetag.Tags),
					Source: map[string]any{
						"arn": arn,
					},
				}

				if !explorer.config.supportResourceFunc(r) {
					slog.Debug("resource is not supported", "cloud_provider", r.CloudProvider, "kind", r.Kind)
					continue
				}

				resources <- r
			}

		}
	}

	return nil
}

func (explorer *Explorer) Close() error { return nil }

func parseTags(tags []resourcetypes.Tag) map[string]string {
	m := make(map[string]string, len(tags))

	for _, t := range tags {
		m[fmt.Sprintf("tag_%s", *t.Key)] = *t.Value
	}

	return m
}
