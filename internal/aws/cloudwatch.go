package aws

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/jellydator/ttlcache/v3"
)

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
		MetricDataQueries: []types.MetricDataQuery{
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

			slog.Debug("cloudwatch cache set for instance", "id", r.ID)
		}
	}

	if item := refiner.cache.Get(r.ID); item != nil {
		r.Source["ec2_instance_cloudwatch_data"] = item.Value()
		return nil
	}

	// No data coming from CloudWatch is already an information. The instance is
	// whether terminated or it's too young to generate monitoring data. We need
	// to cache this information to avoid unecessary queries.
	refiner.cache.Set(r.ID, EC2InstanceCloudwatchRefinedData{CPUUtilizationPercent: 0}, ttlcache.DefaultTTL)

	return nil
}
