// Copyright (C) 2025 dangofish.com
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package aws

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
	"github.com/superdango/cloud-carbon-exporter/internal/must"
	"github.com/superdango/cloud-carbon-exporter/model/cloud"
	"golang.org/x/sync/errgroup"
)

type S3BucketsExplorer struct {
	*Explorer
	mu *sync.Mutex
}

func NewS3BucketsExplorer(explorer *Explorer) *S3BucketsExplorer {
	return &S3BucketsExplorer{
		Explorer: explorer,
		mu:       new(sync.Mutex),
	}
}

func (s3explorer *S3BucketsExplorer) support() string {
	return "s3/bucket"
}

func (s3explorer *S3BucketsExplorer) collectImpacts(ctx cloudcarbonexporter.Context, region string, impacts chan *cloudcarbonexporter.Impact) error {
	if region != "global" {
		slog.Warn("region shoud be 'global' when collecting metric on s3")
		return nil
	}

	s3api := s3.NewFromConfig(s3explorer.awscfg, func(o *s3.Options) {
		o.Region = s3explorer.defaultRegion
	})

	paginator := s3.NewListBucketsPaginator(s3api, &s3.ListBucketsInput{
		MaxBuckets: aws.Int32(100),
	})

	errg := new(errgroup.Group)

	for paginator.HasMorePages() {

		ctx.IncrCalls()
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return &cloudcarbonexporter.ExplorerErr{Err: fmt.Errorf("failed to list buckets: %w", err), Operation: "service/s3:ListBucket"}
		}

		for _, bucket := range output.Buckets {
			bucket := bucket
			errg.Go(func() error {
				var apiErr smithy.APIError
				ctx.IncrCalls()
				s3api := s3.NewFromConfig(s3explorer.awscfg, func(o *s3.Options) {
					o.Region = *bucket.BucketRegion
				})

				// NoSuchTagSet error is expected when buckets have not tags set.
				tagsOutput, err := s3api.GetBucketTagging(ctx, &s3.GetBucketTaggingInput{Bucket: bucket.Name})
				if err != nil && errors.As(err, &apiErr) {
					if apiErr.ErrorCode() != "NoSuchTagSet" {
						return &cloudcarbonexporter.ExplorerErr{Err: fmt.Errorf("failed to get tags for bucket: %s: %w", *bucket.Name, err), Operation: "service/s3:GetBucketTagging"}
					}

					tagsOutput = &s3.GetBucketTaggingOutput{
						TagSet: []types.Tag{},
					}
				}

				size, err := s3explorer.GetBucketSizeBytes(ctx, *bucket.BucketRegion, *bucket.Name)
				if err != nil {
					return fmt.Errorf("failed to get bucket size: %w", err)
				}
				sizeGB := size / 1000 / 1000 / 1000

				slog.Debug("bucket size", "bucket", *bucket.Name, "size_gb", sizeGB)

				impacts <- &cloudcarbonexporter.Impact{
					Energy:            cloud.EstimateObjectStorageWatts(sizeGB),
					EmbodiedEmissions: cloud.EstimateObjectStorageEmbodiedEmissions(sizeGB),
					Labels: cloudcarbonexporter.MergeLabels(
						map[string]string{
							"kind":        "s3/bucket",
							"location":    s3explorer.Region(*bucket.BucketRegion),
							"bucket_name": *bucket.Name,
						},
						parseS3TagList(tagsOutput.TagSet),
					),
				}

				return nil
			})
		}
	}

	return errg.Wait()
}

func (s3explorer *S3BucketsExplorer) load(ctx context.Context) error { return nil }

func (s3explorer *S3BucketsExplorer) GetBucketSizeBytes(ctx cloudcarbonexporter.Context, region string, bucketName string) (float64, error) {
	key := fmt.Sprintf("%s/s3_bucket_%s_size", region, bucketName)

	s3explorer.cache.SetDynamicIfNotExists(ctx, key, func(ctx context.Context) (any, error) {
		return s3explorer.bucketSizeBytes(cloudcarbonexporter.WrapCtx(ctx), bucketName, region)
	}, 12*60*time.Minute) // 12 hours

	entry, err := s3explorer.cache.Get(ctx, key)
	if err != nil {
		return 0.0, fmt.Errorf("failed to list buckets size: %w", err)
	}

	bucketSize, ok := entry.(float64)
	must.Assert(ok, "s3BucketsSize is not a float64")

	return bucketSize, nil
}

// bucketSizeBytes returns the 10 minutes average cpu for all instances in the region
func (s3explorer *S3BucketsExplorer) bucketSizeBytes(ctx cloudcarbonexporter.Context, bucketName string, region string) (float64, error) {
	metricName := "bucket_size_by_bucket_name"

	cwapi := cloudwatch.NewFromConfig(s3explorer.awscfg, func(o *cloudwatch.Options) {
		o.Region = region
	})

	input := &cloudwatch.GetMetricDataInput{
		StartTime: aws.Time(time.Now().AddDate(0, 0, -7)), // from 7 days ago
		EndTime:   aws.Time(time.Now()),                   // to now
		MetricDataQueries: []cwtypes.MetricDataQuery{
			{
				Id: aws.String(metricName),
				MetricStat: &cwtypes.MetricStat{
					Metric: &cwtypes.Metric{
						Namespace:  aws.String("AWS/S3"),
						MetricName: aws.String("BucketSizeBytes"),
						Dimensions: []cwtypes.Dimension{
							{
								Name:  aws.String("BucketName"),
								Value: aws.String(bucketName),
							},
							{
								Name:  aws.String("StorageType"),
								Value: aws.String("StandardStorage"),
							},
						},
					},
					Period: aws.Int32(86400),
					Stat:   aws.String("Average"),
				},
				ReturnData: aws.Bool(true),
			},
		},
	}

	paginator := cloudwatch.NewGetMetricDataPaginator(cwapi, input)

	lastValue := 0.0
	for paginator.HasMorePages() {
		ctx.IncrCalls()
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return 0.0, &cloudcarbonexporter.ExplorerErr{
				Operation: "cloudwatch:GetMetricData",
				Err:       fmt.Errorf("failed to get storage bucket cloudwatch metric in region %s: %w", region, err),
			}

		}

		for _, metricData := range page.MetricDataResults {
			if len(metricData.Values) > 0 {
				lastValue = metricData.Values[len(metricData.Values)-1]
			}
		}

	}

	return lastValue, nil
}

func parseS3TagList(list []types.Tag) map[string]string {
	labels := make(map[string]string)
	for _, t := range list {
		labels[*t.Key] = "tag_" + *t.Value
	}
	return labels
}
