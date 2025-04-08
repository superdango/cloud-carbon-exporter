package gcp

import (
	"context"
	"fmt"
	"sync"
	"time"

	"cloud.google.com/go/storage"
	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
	"github.com/superdango/cloud-carbon-exporter/internal/must"
	"github.com/superdango/cloud-carbon-exporter/model/energy/cloud"
	"google.golang.org/api/iterator"
)

type BucketsExplorer struct {
	*Explorer
	client *storage.Client
	mu     *sync.Mutex
}

func NewBucketsExplorer(ctx context.Context, explorer *Explorer) (bucketsExplorer *BucketsExplorer, err error) {
	bucketsExplorer = &BucketsExplorer{
		Explorer: explorer,
		mu:       new(sync.Mutex),
	}

	bucketsExplorer.client, err = storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create buckets client: %w", err)
	}

	return bucketsExplorer, nil
}

func (bucketsExplorer *BucketsExplorer) collectMetrics(ctx context.Context, metrics chan *cloudcarbonexporter.Metric) error {
	bucketsIter := bucketsExplorer.client.Buckets(ctx, bucketsExplorer.projectID)

	for {
		bucket, err := bucketsIter.Next()
		if err == iterator.Done {
			bucketsExplorer.apiCalls.Add(1)
			break
		}

		if err != nil {
			return fmt.Errorf("failed to iterate on next bucket: %w", err)
		}

		bucketName := bucket.Name
		bucketSize, err := bucketsExplorer.GetBucketSize(ctx, bucketName)
		if err != nil {
			return err
		}

		watts := cloud.EstimateObjectStorage(bytesToGigabytes(bucketSize))

		metrics <- &cloudcarbonexporter.Metric{
			Name: "estimated_watts",
			Labels: cloudcarbonexporter.MergeLabels(
				map[string]string{
					"kind":        "storage/Bucket",
					"bucket_name": bucketName,
					"region":      "global", // TODO: more specific location
				},
				bucket.Labels,
			),
			Value: watts,
		}
	}

	return nil
}

func bytesToGigabytes(bytes float64) float64 {
	const bytesPerGiga float64 = 1024 * 1024 * 1024
	return bytes / bytesPerGiga
}

func (explorer *BucketsExplorer) GetBucketSize(ctx context.Context, bucketName string) (float64, error) {
	explorer.mu.Lock()
	defer explorer.mu.Unlock()

	key := "bucket_size"
	ttl := 5 * time.Minute
	entry, err := explorer.cache.GetOrSet(ctx, key, func(ctx context.Context) (any, error) {
		return explorer.ListBucketSize(ctx)
	}, ttl)
	if err != nil {
		return 0, fmt.Errorf("failed to list bucket size: %w", err)
	}

	bucketsSize, ok := entry.(map[string]float64)
	must.Assert(ok, "bucketsSize is not a map[string]float64")

	bucketSize, found := bucketsSize[bucketName]
	if !found {
		return 0, nil
	}

	return bucketSize, nil
}

func (explorer *BucketsExplorer) ListBucketSize(ctx context.Context) (map[string]float64, error) {
	promqlExpression := `sum by (bucket_name)(avg_over_time(storage_googleapis_com:storage_v2_total_bytes{monitored_resource="gcs_bucket"}[5m]))`
	resolution := 10 * time.Minute

	bucketList, err := explorer.query(ctx, promqlExpression, "bucket_name", resolution)
	if err != nil {
		return nil, fmt.Errorf("failed to query for bucket monitoring data: %w", err)
	}

	explorer.apiCalls.Add(1)

	return bucketList, nil
}
