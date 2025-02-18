package aws

import (
	"context"
	"fmt"
	"reflect"

	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"

	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	resourcetypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"golang.org/x/sync/errgroup"
)

type config struct {
	awscfg  aws.Config
	regions []string
}

type Collector struct {
	config       *config
	refiners     []Refiner
	intensityMap cloudcarbonexporter.CarbonIntensityMap
	energyModel  model
}

type Option func(c *config)

func AWSConfig(cfg aws.Config) Option {
	return func(c *config) {
		c.awscfg = cfg
	}
}

func Regions(regions ...string) Option {
	return func(c *config) {
		c.regions = regions
	}
}

// Refiner adds data on the fly to discovered resource like monitoring or apis
type Refiner interface {
	Refine(ctx context.Context, r *Resource) error
	Supports(r *Resource) bool
}

func NewCollector(ctx context.Context, opts ...Option) *Collector {
	c := &config{
		regions: []string{"eu-west-1", "eu-west-3", "us-west-1"},
	}

	for _, opt := range opts {
		opt(c)
	}

	return &Collector{
		config:       c,
		intensityMap: NewCarbonIntensityMap(),
		energyModel:  newModel(),
		refiners: []Refiner{
			NewEC2InstanceRefiner(c.awscfg),
			NewEC2InstanceCloudwatchRefiner(c.awscfg),
			NewEC2SnapshotRefiner(c.awscfg),
			NewS3BucketRefiner(c.awscfg),
			NewS3BucketCloudwatchRefiner(c.awscfg),
		},
	}
}

type Resource struct {
	cloudcarbonexporter.Resource
	Arn *ARN
}

func (c *Collector) Collect(ctx context.Context, outMetricsCh chan cloudcarbonexporter.Metric) error {
	defer close(outMetricsCh)

	resources := make(chan *Resource)

	errg, errgctx := errgroup.WithContext(ctx)
	errg.SetLimit(5)
	errg.Go(func() error {
		return c.discoverResources(errgctx, resources)
	})

	for {
		select {
		case <-errgctx.Done():
			return errg.Wait()
		case r, ok := <-resources:
			if !ok {
				return errg.Wait()
			}
			errg.Go(func() error {
				if err := c.applyRefiners(errgctx, r); err != nil {
					return err
				}

				c.computeResourceMetrics(r, outMetricsCh)

				return nil
			})
		}
	}
}

// discoverResources list all supported resources that have been tagged in configured regions
func (c *Collector) discoverResources(ctx context.Context, resources chan *Resource) error {
	defer close(resources)

	for _, region := range c.config.regions {
		api := resourcegroupstaggingapi.NewFromConfig(c.config.awscfg, func(o *resourcegroupstaggingapi.Options) {
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

				if !c.energyModel.isSupportedRessource(arn.FullType()) {
					slog.Debug("resource not supported", "type", arn.FullType())
					continue
				}

				resources <- &Resource{
					Arn: arn,
					Resource: cloudcarbonexporter.Resource{
						Kind:     arn.FullType(),
						ID:       arn.ResourceID,
						Location: arn.Region,
						Labels:   parseTags(resourcetag.Tags),
						Source:   make(map[string]any),
					},
				}
			}
		}
	}

	return nil
}

func (c *Collector) computeResourceMetrics(r *Resource, outMetricsCh chan cloudcarbonexporter.Metric) {
	wattMetric := c.energyModel.ComputeResourceEnergyDraw(r)
	if wattMetric == nil {
		return
	}

	outMetricsCh <- *wattMetric
	outMetricsCh <- c.intensityMap.ComputeCO2eq(*wattMetric)
}

func (c *Collector) applyRefiners(ctx context.Context, r *Resource) error {
	for _, refiner := range c.refiners {
		if refiner.Supports(r) {
			err := refiner.Refine(ctx, r)
			if err != nil {
				return fmt.Errorf("failed to refine resource id %s: %w", r.ID, err)
			}
			slog.Debug("resource refined",
				"type", r.Arn.FullType(),
				"id", r.ID,
				"refiner", reflect.TypeOf(refiner))
		}
	}
	return nil
}

func (c *Collector) Close() error {
	return nil
}

func parseTags(tags []resourcetypes.Tag) map[string]string {
	m := make(map[string]string, len(tags))

	for _, t := range tags {
		m[fmt.Sprintf("tag_%s", *t.Key)] = *t.Value
	}

	return m
}
