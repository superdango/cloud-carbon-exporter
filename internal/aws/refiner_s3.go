package aws

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jellydator/ttlcache/v3"
)

type s3InstanceRefinedData struct {
	Region string
}

type s3InstanceRefiner struct {
	mu     *sync.Mutex
	awscfg aws.Config
	cache  *ttlcache.Cache[string, s3InstanceRefinedData]
}

func NewS3BucketRefiner(awscfg aws.Config) *s3InstanceRefiner {
	cache := ttlcache.New(
		ttlcache.WithTTL[string, s3InstanceRefinedData](5 * time.Minute),
	)

	go cache.Start() // starts automatic expired item deletion

	return &s3InstanceRefiner{
		mu:     new(sync.Mutex),
		awscfg: awscfg,
		cache:  cache,
	}
}

func (refiner *s3InstanceRefiner) Supports(r *Resource) bool {
	return r.Kind == "s3"
}

func (refiner *s3InstanceRefiner) Refine(ctx context.Context, r *Resource) error {
	refiner.mu.Lock()
	defer refiner.mu.Unlock()

	if item := refiner.cache.Get(r.ID); item != nil {
		r.Source["s3_bucket_data"] = item.Value()
		r.Region = item.Value().Region
		return nil
	}

	s3api := s3.NewFromConfig(refiner.awscfg, func(o *s3.Options) {
		o.Region = "us-west-1"
	})

	paginator := s3.NewListBucketsPaginator(s3api, &s3.ListBucketsInput{
		MaxBuckets: aws.Int32(100),
	})

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to enrich data for %s resource with id %s: %w", r.Kind, r.ID, err)
		}

		for _, bucket := range output.Buckets {
			refiner.cache.Set(*bucket.Name,
				s3InstanceRefinedData{
					Region: *bucket.BucketRegion,
				},
				ttlcache.DefaultTTL,
			)

		}
	}

	if item := refiner.cache.Get(r.ID); item != nil {
		r.Source["s3_bucket_data"] = item.Value()
		r.Region = item.Value().Region
		r.Source["s3_bucket_data"] = item.Value()
	}

	return nil
}

type S3BucketCloudwatchRefinedData struct {
	BucketSizeBytes float64
}

type S3BucketCloudwatchRefiner struct {
	mu     *sync.Mutex
	awscfg aws.Config
	cache  *ttlcache.Cache[string, S3BucketCloudwatchRefinedData]
}

func NewS3BucketCloudwatchRefiner(awscfg aws.Config) *S3BucketCloudwatchRefiner {
	cache := ttlcache.New(
		ttlcache.WithTTL[string, S3BucketCloudwatchRefinedData](5 * time.Minute),
	)

	go cache.Start() // starts automatic expired item deletion

	return &S3BucketCloudwatchRefiner{
		mu:     new(sync.Mutex),
		awscfg: awscfg,
		cache:  cache,
	}
}

func (refiner *S3BucketCloudwatchRefiner) Supports(r *Resource) bool {
	return r.Kind == "s3"
}

func (refiner *S3BucketCloudwatchRefiner) Refine(ctx context.Context, r *Resource) error {
	refiner.mu.Lock()
	defer refiner.mu.Unlock()

	if item := refiner.cache.Get(r.ID); item != nil {
		r.Source["s3_bucket_cloudwatch_data"] = item.Value()
		return nil
	}

	cwapi := cloudwatch.NewFromConfig(refiner.awscfg, func(o *cloudwatch.Options) {
		o.Region = r.Region
	})

	paginator := cloudwatch.NewGetMetricDataPaginator(cwapi, &cloudwatch.GetMetricDataInput{
		StartTime: aws.Time(time.Now().Add(-10 * time.Minute)),
		EndTime:   aws.Time(time.Now()),
		MetricDataQueries: []cwtypes.MetricDataQuery{
			{
				Id:         aws.String("cpu_utilization_by_instance_id"),
				Expression: aws.String(`SELECT AVG(BucketSizeBytes) FROM "AWS/S3" GROUP BY BucketName`),
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

			slog.Debug("found bucket size monitoring data", "bucket", *m.Label, "bytes", m.Values[0])
			// m.Label correspond to the BucketName
			refiner.cache.Set(*m.Label, S3BucketCloudwatchRefinedData{
				BucketSizeBytes: m.Values[0],
			}, ttlcache.DefaultTTL)

			slog.Debug("cloudwatch cache set for s3 bucket", "id", r.ID)
		}
	}

	if item := refiner.cache.Get(r.ID); item != nil {
		r.Source["s3_bucket_cloudwatch_data"] = item.Value()
		return nil
	}

	// No data coming from CloudWatch is already an information. The bucket it's
	// too young to generate monitoring data. We need to cache this information
	// to avoid unecessary queries but for less longer time to live.
	refiner.cache.Set(r.ID, S3BucketCloudwatchRefinedData{BucketSizeBytes: 0}, 1*time.Minute)

	return nil
}
