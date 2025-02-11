package aws

import (
	"cloudcarbonexporter"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cloudwatchtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	resourcetypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"golang.org/x/sync/errgroup"
)

type config struct {
	awscfg  aws.Config
	regions []string
}

type Collector struct {
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

func NewCollector(ctx context.Context, opts ...Option) *Collector {
	c := &config{
		regions: []string{"eu-west-1", "eu-west-3"},
	}

	for _, opt := range opts {
		opt(c)
	}

	return &Collector{
		config: c,
	}
}

type arn struct {
	namespace string
	service   string
	region    string
	accountID string
	kind      string
	id        string
}

func newArn(arnstr string) (*arn, error) {
	arn := &arn{}

	splitted := strings.Split(arnstr, ":")
	if len(splitted) != 6 {
		return nil, fmt.Errorf("invalid arn format: %s", arnstr)
	}
	arn.namespace = splitted[1]
	arn.service = splitted[2]
	arn.region = splitted[3]
	arn.accountID = splitted[4]

	splittedID := strings.SplitN(splitted[5], "/", 2)
	if len(splittedID) == 1 {
		arn.id = splittedID[0]
		return arn, nil
	}

	if len(splittedID) == 2 {
		arn.kind = splittedID[0]
		arn.id = splittedID[1]
		return arn, nil
	}

	return arn, nil
}

func (a *arn) Type() string {
	if a.kind == "" {
		return a.service
	}

	return fmt.Sprintf("%s/%s", a.service, a.kind)
}

func parseTypeTags(tags []resourcetypes.Tag) map[string]string {
	m := make(map[string]string, len(tags))

	for _, t := range tags {
		m[*t.Key] = *t.Value
	}

	return m
}

func (c *Collector) discoverResources(ctx context.Context, resources chan cloudcarbonexporter.Resource) error {
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
			for _, mapping := range page.ResourceTagMappingList {
				arn, err := newArn(*mapping.ResourceARN)
				if err != nil {
					return fmt.Errorf("failed to parse resource arn: %w", err)
				}

				resources <- cloudcarbonexporter.Resource{
					Kind:     arn.Type(),
					Name:     arn.id,
					Location: arn.region,
					Labels:   parseTypeTags(mapping.Tags),
				}
			}
		}
	}

	return nil
}

func (c *Collector) getResourcesMetrics(ctx context.Context, resources chan cloudcarbonexporter.Resource) error {

	return nil
}

func (c *Collector) Collect(ctx context.Context, metrics chan cloudcarbonexporter.Metric) error {
	defer close(metrics)

	resources := make(chan cloudcarbonexporter.Resource)

	errg, errgctx := errgroup.WithContext(ctx)

	errg.Go(func() error {
		return c.discoverResources(errgctx, resources)
	})

	err := errg.Wait()
	if err != nil {
		return err
	}
	cwapi := cloudwatch.NewFromConfig(c.config.awscfg)
	paginator := cloudwatch.NewListMetricsPaginator(cwapi, &cloudwatch.ListMetricsInput{RecentlyActive: cloudwatchtypes.RecentlyActivePt3h})

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list cloudwatch metrics: %w", err)
		}

		for _, m := range output.Metrics {
			dim, _ := json.Marshal(m.Dimensions)
			fmt.Println(*m.MetricName, *m.Namespace, string(dim))
		}
	}
	return nil
}

func (c *Collector) Close() error {
	return nil
}
