package aws

import (
	"context"
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
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

const day = 24 * time.Hour

type Explorer struct {
	mu             *sync.Mutex
	accountID      string
	awscfg         aws.Config
	awsbilling     aws.Config
	defaultRegion  string
	roleArn        string
	billingRoleArn string
	accountAZs     []AvailabilityZone
	services       map[string][]string
}

type Option func(*Explorer)

func WithAWSConfig(cfg aws.Config) Option {
	return func(e *Explorer) {
		e.awscfg = cfg
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
		explorer.awscfg.Credentials = aws.NewCredentialsCache(
			stscreds.NewAssumeRoleProvider(sts.NewFromConfig(
				explorer.awscfg,
				func(o *sts.Options) { o.Region = explorer.defaultRegion },
			), explorer.roleArn),
		)
		slog.Info("assuming aws role for resource services api calls", "role", explorer.roleArn)
	}

	go explorer.accountInfosRefresher(ctx)
	go explorer.accountServicesRefresher(ctx)

	return explorer, nil
}

func (explorer *Explorer) refreshAccountID(ctx context.Context) error {
	start := time.Now()
	defer func() {
		slog.Info("aws account id refreshed", "duration_ms", time.Since(start).Milliseconds())
	}()

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

	return nil
}

// discoverResources list all supported resources that have been tagged in configured regions
func (explorer *Explorer) Find(ctx context.Context, resources chan *cloudcarbonexporter.Resource) error {

	errg, errgctx := errgroup.WithContext(ctx)
	for service, regions := range explorer.services {
		for _, region := range regions {
			region := region
			switch service {
			case "Amazon Elastic Compute Cloud - Compute":
				errg.Go(func() error {
					return explorer.listRegionEC2Instances(errgctx, region, resources)
				})
			}
		}
	}

	return errg.Wait()

}

func (explorer *Explorer) Close() error { return nil }

func (explorer *Explorer) IsReady() bool {
	availabilityZonesAreLoaded := len(explorer.accountAZs) > 0
	accountIDIsLoaded := explorer.accountID != ""

	return availabilityZonesAreLoaded && accountIDIsLoaded
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

func (e *Explorer) refreshAccountAvailibilityZones(ctx context.Context) error {
	start := time.Now()
	defer func() {
		slog.Info("aws account availibility zones refreshed", "duration_ms", time.Since(start).Milliseconds())
	}()

	ec2api := ec2.NewFromConfig(e.awscfg, func(o *ec2.Options) {
		o.Region = e.defaultRegion
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
			ec2api := ec2.NewFromConfig(e.awscfg, func(o *ec2.Options) {
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

func (e *Explorer) accountInfosRefresher(ctx context.Context) {
	every := time.Hour
	wait := must.NewWait(30 * time.Second)
	errg, errgctx := errgroup.WithContext(ctx)
	for {
		errg.Go(func() error {
			err := e.refreshAccountAvailibilityZones(errgctx)
			if err != nil {
				return fmt.Errorf("failed to refresh aws account availability zones: %w", err)
			}
			return nil
		})

		errg.Go(func() error {
			err := e.refreshAccountID(errgctx)
			if err != nil {
				return fmt.Errorf("failed to refresh aws account id: %w", err)
			}
			return nil
		})

		if err := errg.Wait(); err != nil {
			slog.Error("failed to refresh infos", "err", err)
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

func (e *Explorer) accountServicesRefresher(ctx context.Context) {
	every := time.Hour
	wait := must.NewWait(30 * time.Second)
	for {
		if !e.IsReady() {
			wait.Static(time.Second)
			continue
		}
		if err := e.refreshAccountServices(ctx); err != nil {
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

func (e *Explorer) refreshAccountServices(ctx context.Context) error {
	start := time.Now()
	defer func() {
		slog.Info("aws account services refreshed", "duration_ms", time.Since(start).Milliseconds())
	}()

	costs := costexplorer.NewFromConfig(e.awsbilling, func(o *costexplorer.Options) {
		o.Region = e.defaultRegion
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

	services := make(map[string][]string, 0)
	for _, result := range output.ResultsByTime {
		for _, group := range result.Groups {
			service, az := group.Keys[0], group.Keys[1]
			services[service] = append(services[service], e.Region(az))
		}
	}

	for service := range services {
		slices.Sort(services[service])
		services[service] = slices.Compact(services[service])
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	e.services = services

	return nil
}
