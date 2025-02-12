package aws

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
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
