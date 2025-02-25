package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
	"github.com/superdango/cloud-carbon-exporter/internal/must"
	"golang.org/x/sync/errgroup"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	resourcetypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

const day = 24 * time.Hour

type Explorer struct {
	mu             *sync.Mutex
	accountID      string
	aws            aws.Config
	awsbilling     aws.Config
	defaultRegion  string
	roleArn        string
	billingRoleArn string
	accountAZs     []AvailabilityZone
}

type Option func(*Explorer)

func WithAWSConfig(cfg aws.Config) Option {
	return func(e *Explorer) {
		e.aws = cfg
		e.awsbilling = cfg
	}
}

func WithDefaultRegion(region string) Option {
	return func(c *Explorer) {
		c.defaultRegion = region
	}
}

func WithRoleArn(role string) Option {
	return func(c *Explorer) {
		c.roleArn = role
	}
}

func WithBillingRoleArn(role string) Option {
	return func(c *Explorer) {
		c.billingRoleArn = role
	}
}

func NewExplorer(ctx context.Context, opts ...Option) (explorer *Explorer, err error) {
	explorer = &Explorer{
		mu:            new(sync.Mutex),
		defaultRegion: "us-east-1",
		accountAZs:    make([]AvailabilityZone, 0),
	}

	for _, opt := range opts {
		if opt != nil {
			opt(explorer)
		}
	}

	if explorer.billingRoleArn != "" {
		explorer.awsbilling.Credentials = aws.NewCredentialsCache(
			stscreds.NewAssumeRoleProvider(sts.NewFromConfig(
				explorer.awsbilling,
				func(o *sts.Options) { o.Region = explorer.defaultRegion },
			), explorer.billingRoleArn,
			),
		)
		slog.Info("assuming aws role for billing api calls", "role", explorer.billingRoleArn)
	}

	if explorer.roleArn != "" {
		explorer.aws.Credentials = aws.NewCredentialsCache(
			stscreds.NewAssumeRoleProvider(sts.NewFromConfig(
				explorer.aws,
				func(o *sts.Options) { o.Region = explorer.defaultRegion },
			), explorer.roleArn),
		)
		slog.Info("assuming aws role for resource services api calls", "role", explorer.roleArn)
	}

	go explorer.refreshAccountAZs(ctx, explorer.aws, explorer.defaultRegion, time.Hour)
	go explorer.refreshAccountID(ctx)

	return explorer, nil
}

func (explorer *Explorer) refreshAccountID(ctx context.Context) error {
	stsapi := sts.NewFromConfig(explorer.awsbilling, func(o *sts.Options) {
		o.Region = explorer.defaultRegion
	})

	output, err := stsapi.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return fmt.Errorf("failed to get caller identity: %w", err)
	}

	explorer.mu.Lock()
	defer explorer.mu.Unlock()

	explorer.accountID = *output.Account

	slog.Info("explorer account id is refreshed", "account_id", explorer.accountID)
	return nil
}

// discoverResources list all supported resources that have been tagged in configured regions
func (explorer *Explorer) Find(ctx context.Context, resources chan *cloudcarbonexporter.Resource) error {
	costs := costexplorer.NewFromConfig(explorer.awsbilling, func(o *costexplorer.Options) {
		o.Region = explorer.defaultRegion
	})

	output, err := costs.GetCostAndUsage(ctx, &costexplorer.GetCostAndUsageInput{
		TimePeriod: &cetypes.DateInterval{
			Start: aws.String(time.Now().Add(-7 * day).Format(time.DateOnly)),
			End:   aws.String(time.Now().Format(time.DateOnly)),
		},
		Granularity: cetypes.GranularityDaily,
		Metrics:     []string{"UsageQuantity"},
		GroupBy: []cetypes.GroupDefinition{
			{
				Key:  aws.String("SERVICE"),
				Type: cetypes.GroupDefinitionTypeDimension,
			},
			{
				Key:  aws.String("AZ"),
				Type: cetypes.GroupDefinitionTypeDimension,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to get cost and usage for aws account: %w", err)
	}

	list := make(map[string][]string, 0)
	for _, result := range output.ResultsByTime {
		for _, group := range result.Groups {
			service, az := group.Keys[0], group.Keys[1]
			list[service] = append(list[service], explorer.Region(az))
		}
	}
	for service := range list {
		slices.Sort(list[service])
		list[service] = slices.Compact(list[service])
	}

	jsn, _ := json.MarshalIndent(list, "", "  ")
	fmt.Println(string(jsn))

	//explorer.listRegionEC2Instances(ctx, "eu-west-3", nil)

	// freetierclient := freetier.NewFromConfig(explorer.billingcfg, func(o *freetier.Options) {
	// 	o.Region = explorer.defaultRegion
	// })

	// paginator := freetier.NewGetFreeTierUsagePaginator(freetierclient, &freetier.GetFreeTierUsageInput{})

	// for paginator.HasMorePages() {
	// 	freetieroutput, err := paginator.NextPage(ctx)
	// 	if err != nil {
	// 		return fmt.Errorf("failed to get free tier usage for aws account: %w", err)
	// 	}

	// 	for _, u := range freetieroutput.FreeTierUsages {
	// 		fmt.Println(*u.Service, *u.Region, " usage", u.ActualUsageAmount, "/", int(u.Limit), *u.Unit)
	// 	}
	// }

	return nil //explorer.listEC2Instances(ctx, resources, "eu-west-1", "eu-west-3")

}

func (explorer *Explorer) Close() error { return nil }

func (explorer *Explorer) IsReady() bool {
	availabilityZonesAreLoaded := len(explorer.accountAZs) > 0
	accountIDIsLoaded := explorer.accountID != ""

	fmt.Println(availabilityZonesAreLoaded, accountIDIsLoaded)
	return availabilityZonesAreLoaded && accountIDIsLoaded
}

func (explorer *Explorer) listEC2Instances(ctx context.Context, resources chan *cloudcarbonexporter.Resource, regions ...string) error {
	errg, errgctx := errgroup.WithContext(ctx)

	for _, region := range regions {
		region := region
		errg.Go(func() error {
			return explorer.listRegionEC2Instances(errgctx, region, resources)
		})
	}

	return errg.Wait()
}

func (explorer *Explorer) listRegionEC2Instances(ctx context.Context, region string, resources chan *cloudcarbonexporter.Resource) error {
	ec2api := ec2.NewFromConfig(explorer.aws, func(o *ec2.Options) {
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

func parseTags(tags []resourcetypes.Tag) map[string]string {
	m := make(map[string]string, len(tags))

	for _, t := range tags {
		m[fmt.Sprintf("tag_%s", *t.Key)] = *t.Value
	}

	return m
}

type AvailabilityZone struct {
	Name   string
	Region string
}

func (e *Explorer) IsZone(zone string) bool {
	for _, az := range e.accountAZs {
		if az.Name == zone {
			return true
		}
	}
	return false
}

func (e *Explorer) IsRegion(region string) bool {
	for _, az := range e.accountAZs {
		if az.Region == region {
			return true
		}
	}
	return false
}

// Region return "global" is location is unknown, itself if location is already a region or the
// parent region if location is an availability zone
func (e *Explorer) Region(location string) (region string) {
	for _, az := range e.accountAZs {
		if location == az.Name || location == az.Region {
			return az.Region
		}
	}
	return "global"
}

func (e *Explorer) Refresh(ctx context.Context, awscfg aws.Config, defaultRegion string) error {
	start := time.Now()
	defer func() {
		slog.Info("aws account availibility zones refreshed", "duration_ms", time.Since(start).Milliseconds())
	}()

	ec2api := ec2.NewFromConfig(awscfg, func(o *ec2.Options) {
		o.Region = defaultRegion
	})

	regions, err := ec2api.DescribeRegions(ctx, &ec2.DescribeRegionsInput{})
	if err != nil {
		return fmt.Errorf("failed to describe account regions: %w", err)
	}

	errg, errgctx := errgroup.WithContext(ctx)

	mu := new(sync.Mutex)
	azs := make([]AvailabilityZone, 0)

	for _, region := range regions.Regions {
		region := *region.RegionName

		errg.Go(func() error {
			ec2api := ec2.NewFromConfig(awscfg, func(o *ec2.Options) {
				o.Region = region
			})

			zones, err := ec2api.DescribeAvailabilityZones(errgctx, &ec2.DescribeAvailabilityZonesInput{})
			if err != nil {
				return fmt.Errorf("failed to describe account availability zones: %w", err)
			}

			mu.Lock()
			defer mu.Unlock()
			for _, az := range zones.AvailabilityZones {
				azs = append(azs, AvailabilityZone{
					Name:   *az.ZoneName,
					Region: *az.RegionName,
				})
			}
			return nil
		})
	}

	if err := errg.Wait(); err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	e.accountAZs = azs

	return nil
}

func (e *Explorer) refreshAccountAZs(ctx context.Context, awscfg aws.Config, defaultRegion string, every time.Duration) {
	wait := must.NewWait(30 * time.Second)
	for {
		if err := e.Refresh(ctx, awscfg, defaultRegion); err != nil {
			slog.Warn("failed to refresh aws account availibility zones", "err", err)
			wait.Linearly(time.Second)
			continue
		}
		wait.Reset()

		select {
		case <-ctx.Done():
			return
		case <-time.Tick(every):
			continue
		}
	}
}
