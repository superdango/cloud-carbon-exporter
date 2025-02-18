package aws

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/superdango/cloud-carbon-exporter"


	
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
	config   *config
	refiners []Refiner
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

// Refiner adds data on the fly to discovered resource like monitoring or apis
type Refiner interface {
	Refine(ctx context.Context, r *Resource) error
	Supports(r *Resource) bool
}

func NewCollector(ctx context.Context, opts ...Option) *Collector {
	c := &config{
		regions: []string{"eu-west-1", "eu-west-3"},
	}

	for _, opt := range opts {
		opt(c)
	}

	return &Collector{
		config: c,
		refiners: []Refiner{
			NewEC2InstanceRefiner(c.awscfg),
			NewEC2InstanceCloudwatchRefiner(c.awscfg),
		},
	}
}

type ARN struct {
	Partition    string
	Service      string
	Region       string
	AccountID    string
	ResourceType string
	ResourceID   string
}

func NewARN(arnstr string) (*ARN, error) {
	arn := &ARN{}

	splitted := strings.Split(arnstr, ":")
	if len(splitted) != 6 {
		return nil, fmt.Errorf("invalid arn format: %s", arnstr)
	}
	arn.Partition = splitted[1]
	arn.Service = splitted[2]
	arn.Region = splitted[3]
	arn.AccountID = splitted[4]

	resourceComponent := strings.SplitN(splitted[5], "/", 2)
	switch len(resourceComponent) {
	case 1:
		arn.ResourceID = resourceComponent[0]
		return arn, nil
	case 2:
		arn.ResourceType = resourceComponent[0]
		arn.ResourceID = resourceComponent[1]
		return arn, nil
	default:
		return nil, fmt.Errorf("invalid arn resource id format: %s", splitted[5])
	}
}

func (a *ARN) FullType() string {
	if a.ResourceType == "" {
		return a.Service
	}

	return fmt.Sprintf("%s/%s", a.Service, a.ResourceType)
}

func parseTypeTags(tags []resourcetypes.Tag) map[string]string {
	m := make(map[string]string, len(tags))

	for _, t := range tags {
		m[fmt.Sprintf("tag_%s", *t.Key)] = *t.Value
	}

	return m
}

type Resource struct {
	cloudcarbonexporter.Resource
	Arn *ARN
}

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

				if !getModel().isSupportedRessource(arn.FullType()) {
					slog.Debug("unsupported resource", "type", arn.FullType())
					continue
				}

				resources <- &Resource{
					Arn: arn,
					Resource: cloudcarbonexporter.Resource{
						Kind:     arn.FullType(),
						ID:       arn.ResourceID,
						Location: arn.Region,
						Labels:   parseTypeTags(resourcetag.Tags),
						Source:   make(map[string]any),
					},
				}
			}
		}
	}

	return nil
}

func (c *Collector) Collect(ctx context.Context, metrics chan cloudcarbonexporter.Metric) error {
	defer close(metrics)

	resources := make(chan *Resource)

	errg, errgctx := errgroup.WithContext(ctx)
	errg.SetLimit(5)
	errg.Go(func() error {
		return c.discoverResources(errgctx, resources)
	})

	for {
		select {
		case <-errgctx.Done():
			return errgctx.Err()
		case r, ok := <-resources:
			if !ok {
				return errg.Wait()
			}
			errg.Go(func() error {
				if err := c.applyRefiners(errgctx, r); err != nil {
					return err
				}

				getModel()[r.Arn.FullType()](r, metrics)

				return nil
			})
		}
	}
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
