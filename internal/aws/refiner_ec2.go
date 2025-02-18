package aws

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/jellydator/ttlcache/v3"
)

type EC2InstanceRefinedData struct {
	CPU     int
	Running bool
}

type EC2InstanceRefiner struct {
	mu     *sync.Mutex
	awscfg aws.Config
	cache  *ttlcache.Cache[string, EC2InstanceRefinedData]
}

func NewEC2InstanceRefiner(awscfg aws.Config) *EC2InstanceRefiner {
	cache := ttlcache.New(
		ttlcache.WithTTL[string, EC2InstanceRefinedData](5 * time.Minute),
	)

	go cache.Start() // starts automatic expired item deletion

	return &EC2InstanceRefiner{
		mu:     new(sync.Mutex),
		awscfg: awscfg,
		cache:  cache,
	}
}

func (refiner *EC2InstanceRefiner) Supports(r *Resource) bool {
	return r.Kind == "ec2/instance"
}

func (refiner *EC2InstanceRefiner) Refine(ctx context.Context, r *Resource) error {
	refiner.mu.Lock()
	defer refiner.mu.Unlock()

	if item := refiner.cache.Get(r.ID); item != nil {
		r.Source["ec2_instance_data"] = item.Value()
		return nil
	}

	ec2api := ec2.NewFromConfig(refiner.awscfg, func(o *ec2.Options) {
		o.Region = r.Arn.Region
	})

	paginator := ec2.NewDescribeInstancesPaginator(ec2api, &ec2.DescribeInstancesInput{
		MaxResults: aws.Int32(100),
	})

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to enrich data for %s resource with id %s: %w", r.Kind, r.ID, err)
		}

		for _, reservation := range output.Reservations {
			for _, instance := range reservation.Instances {
				refiner.cache.Set(*instance.InstanceId,
					EC2InstanceRefinedData{
						CPU:     int(*instance.CpuOptions.CoreCount),
						Running: instance.State.Name == types.InstanceStateNameRunning,
					},
					ttlcache.DefaultTTL,
				)
			}
		}
	}

	if item := refiner.cache.Get(r.ID); item != nil {
		r.Source["ec2_instance_data"] = item.Value()
	}

	return nil
}

type EC2InstanceCloudwatchRefinedData struct {
	CPUUtilizationPercent float64
}

type EC2InstanceCloudwatchRefiner struct {
	mu     *sync.Mutex
	awscfg aws.Config
	cache  *ttlcache.Cache[string, EC2InstanceCloudwatchRefinedData]
}

func NewEC2InstanceCloudwatchRefiner(awscfg aws.Config) *EC2InstanceCloudwatchRefiner {
	cache := ttlcache.New(
		ttlcache.WithTTL[string, EC2InstanceCloudwatchRefinedData](5 * time.Minute),
	)

	go cache.Start() // starts automatic expired item deletion

	return &EC2InstanceCloudwatchRefiner{
		mu:     new(sync.Mutex),
		awscfg: awscfg,
		cache:  cache,
	}
}

func (refiner *EC2InstanceCloudwatchRefiner) Supports(r *Resource) bool {
	return r.Kind == "ec2/instance"
}

func (refiner *EC2InstanceCloudwatchRefiner) Refine(ctx context.Context, r *Resource) error {
	refiner.mu.Lock()
	defer refiner.mu.Unlock()

	if item := refiner.cache.Get(r.ID); item != nil {
		r.Source["ec2_instance_cloudwatch_data"] = item.Value()
		return nil
	}

	cwapi := cloudwatch.NewFromConfig(refiner.awscfg, func(o *cloudwatch.Options) {
		o.Region = r.Arn.Region
	})

	paginator := cloudwatch.NewGetMetricDataPaginator(cwapi, &cloudwatch.GetMetricDataInput{
		StartTime: aws.Time(time.Now().Add(-10 * time.Minute)),
		EndTime:   aws.Time(time.Now()),
		MetricDataQueries: []cwtypes.MetricDataQuery{
			{
				Id:         aws.String("cpu_utilization_by_instance_id"),
				Expression: aws.String(`SELECT AVG(CPUUtilization) FROM "AWS/EC2" GROUP BY InstanceId`),
				Period:     aws.Int32(60 * 10 /*10 minutes*/),
			},
		},
	})

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list ec2 instance cloudwatch metrics: %w", err)
		}

		for _, m := range output.MetricDataResults {
			if len(m.Values) == 0 {
				continue
			}
			// m.Label correspond to the InstanceID
			refiner.cache.Set(*m.Label, EC2InstanceCloudwatchRefinedData{
				CPUUtilizationPercent: m.Values[0],
			}, ttlcache.DefaultTTL)
		}
	}

	if item := refiner.cache.Get(r.ID); item != nil {
		r.Source["ec2_instance_cloudwatch_data"] = item.Value()
		return nil
	}

	// No data coming from CloudWatch is already an information. The instance is
	// whether terminated or it's too young to generate monitoring data. We need
	// to cache this information to avoid unecessary queries but for less longer
	// time to live.
	refiner.cache.Set(r.ID, EC2InstanceCloudwatchRefinedData{CPUUtilizationPercent: 0}, 1*time.Minute)

	return nil
}

type EC2SnapshotRefinedData struct {
	SizeBytes float64
}

type EC2SnapshotRefiner struct {
	mu     *sync.Mutex
	awscfg aws.Config
	cache  *ttlcache.Cache[string, EC2SnapshotRefinedData]
}

func NewEC2SnapshotRefiner(awscfg aws.Config) *EC2SnapshotRefiner {
	cache := ttlcache.New(
		ttlcache.WithTTL[string, EC2SnapshotRefinedData](5 * time.Minute),
	)

	go cache.Start() // starts automatic expired item deletion

	return &EC2SnapshotRefiner{
		mu:     new(sync.Mutex),
		awscfg: awscfg,
		cache:  cache,
	}
}

func (refiner *EC2SnapshotRefiner) Supports(r *Resource) bool {
	return r.Kind == "ec2/snapshot"
}

func (refiner *EC2SnapshotRefiner) Refine(ctx context.Context, r *Resource) error {
	refiner.mu.Lock()
	defer refiner.mu.Unlock()

	if item := refiner.cache.Get(r.ID); item != nil {
		r.Source["ec2_snapshot_data"] = item.Value()
		return nil
	}

	ec2api := ec2.NewFromConfig(refiner.awscfg, func(o *ec2.Options) {
		o.Region = r.Arn.Region
	})

	paginator := ec2.NewDescribeSnapshotsPaginator(ec2api, &ec2.DescribeSnapshotsInput{
		MaxResults: aws.Int32(100),
		OwnerIds:   []string{"self"},
	})

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to enrich data for %s resource with id %s: %w", r.Kind, r.ID, err)
		}

		for _, snapshot := range output.Snapshots {
			refiner.cache.Set(*snapshot.SnapshotId,
				EC2SnapshotRefinedData{
					SizeBytes: float64(*snapshot.FullSnapshotSizeInBytes),
				},
				ttlcache.DefaultTTL,
			)

		}
	}

	if item := refiner.cache.Get(r.ID); item != nil {
		r.Source["ec2_snapshot_data"] = item.Value()
	}

	return nil
}
