/*
 *   Copyright (c) 2025
 *   All rights reserved.
 */
package aws

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
	"github.com/superdango/cloud-carbon-exporter/internal/cache"
	"github.com/superdango/cloud-carbon-exporter/model/carbon"
	"github.com/superdango/cloud-carbon-exporter/model/energy/primitives"
	"golang.org/x/sync/errgroup"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

const DAY = 24 * time.Hour

type subExplorer interface {
	collectMetrics(ctx context.Context, region string, metrics chan *cloudcarbonexporter.Metric) error
	load(ctx context.Context) error
	support() string
}

type Explorer struct {
	mu                 *sync.Mutex
	awscfg             aws.Config
	cache              *cache.Memory
	defaultRegion      string
	roleArn            string
	accountAZs         []AvailabilityZone
	activeServices     map[string][]string // serviceName: [region1, region2, ...]
	subExplorers       map[string][]subExplorer
	carbonIntensityMap carbon.IntensityMap
	instanceTypeInfos  map[string]instanceTypeInfos
	apiCallsCounter    *atomic.Int64
}

type ExplorerOption func(*Explorer)

func WithAWSConfig(cfg aws.Config) ExplorerOption {
	return func(e *Explorer) {
		e.awscfg = cfg
	}
}

func WithDefaultRegion(region string) ExplorerOption {
	return func(c *Explorer) {
		c.defaultRegion = region
	}
}

func WithRoleArn(role string) ExplorerOption {
	return func(c *Explorer) {
		c.roleArn = role
	}
}

func NewExplorer() *Explorer {
	explorer := &Explorer{
		mu:                 new(sync.Mutex),
		defaultRegion:      "us-east-1",
		accountAZs:         make([]AvailabilityZone, 0),
		carbonIntensityMap: carbon.NewAWSCloudCarbonFootprintIntensityMap(),
		instanceTypeInfos:  make(map[string]instanceTypeInfos),
		apiCallsCounter:    new(atomic.Int64),
	}

	explorer.subExplorers = map[string][]subExplorer{
		"Amazon Elastic Compute Cloud - Compute": {
			NewEC2InstanceExplorer(explorer),
			NewEC2VolumeExplorer(explorer),
		},
		"Amazon Relational Database Service": {
			NewRDSInstanceExplorer(explorer),
		},
	}

	return explorer
}

func (explorer *Explorer) SupportedServices() []string {
	supportedServices := make([]string, 0)

	for _, subexplorers := range explorer.subExplorers {
		for _, subexplorer := range subexplorers {
			supportedServices = append(supportedServices, subexplorer.support())
		}
	}

	return supportedServices
}

func (explorer *Explorer) Configure(opts ...ExplorerOption) *Explorer {
	for _, opt := range opts {
		if opt != nil {
			opt(explorer)
		}
	}

	return explorer
}

// NewExplorer initialize and returns a new AWS Explorer.
func (explorer *Explorer) Init(ctx context.Context) (err error) {
	explorer.cache = cache.NewMemory(ctx, 5*time.Minute)

	if explorer.roleArn != "" {
		explorer.awscfg.Credentials = aws.NewCredentialsCache(
			stscreds.NewAssumeRoleProvider(sts.NewFromConfig(
				explorer.awscfg,
				func(o *sts.Options) { o.Region = explorer.defaultRegion },
			), explorer.roleArn),
		)
		slog.Info("assuming aws role for resource services api calls", "role", explorer.roleArn)
	}

	errg, errgctx := errgroup.WithContext(ctx)

	errg.Go(func() error {
		return explorer.loadSubExplorers(errgctx)
	})

	errg.Go(func() error {
		return explorer.discoverActiveServicesAndRegions(errgctx)
	})

	return errg.Wait()
}

func (explorer *Explorer) loadSubExplorers(ctx context.Context) error {
	explorer.subExplorers = map[string][]subExplorer{
		"Amazon Elastic Compute Cloud - Compute": {
			NewEC2InstanceExplorer(explorer),
			NewEC2VolumeExplorer(explorer),
		},
		"Amazon Relational Database Service": {
			NewRDSInstanceExplorer(explorer),
		},
	}

	errg, errgctx := errgroup.WithContext(ctx)
	for _, energyEstimators := range explorer.subExplorers {
		for _, energyEstimator := range energyEstimators {
			energyEstimator := energyEstimator
			errg.Go(func() error {
				if err := energyEstimator.load(errgctx); err != nil {
					return fmt.Errorf("failed to load resources creator: %w", err)
				}
				return nil
			})
		}
	}
	return errg.Wait()
}

// CollectMetrics resources on the configured AWS Account and sends them in the resources chan
func (explorer *Explorer) CollectMetrics(ctx context.Context, metrics chan *cloudcarbonexporter.Metric, errs chan error) {
	explorer.apiCallsCounter.Store(0) // reset api calls counter
	energyMetrics := make(chan *cloudcarbonexporter.Metric)

	wg := new(sync.WaitGroup)
	wg.Add(1)

	go func() {
		defer wg.Done()
		for energyMetric := range energyMetrics {
			energyMetric.SetLabel("cloud_provider", "aws")
			energyMetric.Value *= primitives.GoodPUE
			metrics <- energyMetric
			metrics <- explorer.carbonIntensityMap.ComputeCO2eq(energyMetric)
		}
	}()

	go func() {
		defer close(energyMetrics)
		explorer.collectMetrics(ctx, energyMetrics, errs)
	}()

	wg.Wait()

	metrics <- &cloudcarbonexporter.Metric{
		Name: "api_calls",
		Labels: map[string]string{
			"provider": "aws",
		},
		Value: float64(explorer.apiCallsCounter.Load()),
	}
}

func (explorer *Explorer) collectMetrics(ctx context.Context, metrics chan *cloudcarbonexporter.Metric, errs chan error) {
	wg := new(sync.WaitGroup)
	for service, regions := range explorer.activeServices {
		for _, region := range regions {
			for _, subExplorer := range explorer.subExplorers[service] {
				region := region
				collector := subExplorer
				wg.Add(1)
				go func() {
					defer wg.Done()
					errs <- collector.collectMetrics(ctx, region, metrics)
				}()
			}
		}
	}

	wg.Wait()
}

// Close do nothing else but implementing the Explorer interface
func (explorer *Explorer) Close() error { return nil }

// IsReady returns true if explorer can effectively return resources
func (explorer *Explorer) IsReady() bool {
	availabilityZonesAreLoaded := len(explorer.accountAZs) > 0

	return availabilityZonesAreLoaded
}

// AvailabilityZone represents an AWS Availability Zone with its name and
// the region containing it.
type AvailabilityZone struct {
	Name   string
	Region string
}

// IsValidZone returns true if zone is a valid zone
func (e *Explorer) IsValidZone(zone string) bool {
	for _, az := range e.accountAZs {
		if az.Name == zone {
			return true
		}
	}
	return false
}

// IsValidRegion returns true if region is a valid region
func (e *Explorer) IsValidRegion(region string) bool {
	for _, az := range e.accountAZs {
		if az.Region == region {
			return true
		}
	}
	return false
}

// Region returns "global" is location is unknown, itself if location is already a region or the
// parent region if location is an availability zone
func (e *Explorer) Region(location string) (region string) {
	for _, az := range e.accountAZs {
		if location == az.Name || location == az.Region {
			return az.Region
		}
	}
	return "global"
}

// getAWSAccountID returns the account id targeted by the awscfg
func (explorer *Explorer) getAWSAccountID(ctx context.Context, awscfg aws.Config) (accountID string, err error) {
	start := time.Now()
	defer func() {
		slog.Info("got aws account id from caller identity ", "duration_ms", time.Since(start).Milliseconds())
	}()

	stsapi := sts.NewFromConfig(awscfg, func(o *sts.Options) {
		o.Region = explorer.defaultRegion
	})

	explorer.apiCallsCounter.Add(1)
	output, err := stsapi.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", fmt.Errorf("failed to get caller identity: %w", err)
	}

	return *output.Account, nil
}

// discoverActiveServicesAndRegions first looks for the targeted account id and initialize
// all AWS AZs / Regions. Then, it discovers active services and regions via cost explorer
// apis.
func (explorer *Explorer) discoverActiveServicesAndRegions(ctx context.Context) error {
	start := time.Now()
	defer func() {
		slog.Info("aws account services refreshed", "duration_ms", time.Since(start).Milliseconds())
	}()

	accountID, err := explorer.getAWSAccountID(ctx, explorer.awscfg)
	if err != nil {
		return fmt.Errorf("failed to retreive target account id: %w", err)
	}

	err = explorer.refreshAccountAvailibilityZones(ctx)
	if err != nil {
		return fmt.Errorf("failed to update list of aws availability zones: %w", err)
	}

	costs := costexplorer.NewFromConfig(explorer.awscfg, func(o *costexplorer.Options) {
		o.Region = explorer.defaultRegion
	})

	explorer.apiCallsCounter.Add(1)
	output, err := costs.GetCostAndUsage(ctx, &costexplorer.GetCostAndUsageInput{
		TimePeriod: &cetypes.DateInterval{
			Start: aws.String(time.Now().Add(-7 * DAY).Format(time.DateOnly)),
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
		Filter: &cetypes.Expression{
			Dimensions: &cetypes.DimensionValues{
				Key:    cetypes.DimensionLinkedAccount,
				Values: []string{accountID},
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
			services[service] = append(services[service], explorer.Region(az))
		}
	}

	for service := range services {
		slices.Sort(services[service])
		services[service] = slices.Compact(services[service])
	}

	explorer.mu.Lock()
	defer explorer.mu.Unlock()

	explorer.activeServices = services

	for service, locations := range services {
		slog.Debug("discovered service", "service", service, "locations", locations)
	}

	return nil
}

func (explorer *Explorer) refreshAccountAvailibilityZones(ctx context.Context) error {
	start := time.Now()
	defer func() {
		slog.Info("aws account availibility zones refreshed", "duration_ms", time.Since(start).Milliseconds())
	}()

	ec2api := ec2.NewFromConfig(explorer.awscfg, func(o *ec2.Options) {
		o.Region = explorer.defaultRegion
	})

	explorer.apiCallsCounter.Add(1)
	regions, err := ec2api.DescribeRegions(ctx, &ec2.DescribeRegionsInput{})
	if err != nil {
		return &cloudcarbonexporter.ExplorerErr{Err: fmt.Errorf("failed to describe account regions: %w", err), Operation: "service/ec2:DescribeRegions"}
	}

	errg, errgctx := errgroup.WithContext(ctx)

	mu := new(sync.Mutex)
	azs := make([]AvailabilityZone, 0)

	for _, region := range regions.Regions {
		region := *region.RegionName

		errg.Go(func() error {
			ec2api := ec2.NewFromConfig(explorer.awscfg, func(o *ec2.Options) {
				o.Region = region
			})

			explorer.apiCallsCounter.Add(1)
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

	explorer.mu.Lock()
	defer explorer.mu.Unlock()
	explorer.accountAZs = azs

	return nil
}
