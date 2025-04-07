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
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
	"github.com/superdango/cloud-carbon-exporter/internal/cache"
	"github.com/superdango/cloud-carbon-exporter/model/energy/cloud"
)

type EC2VolumeEstimator struct {
	awscfg            aws.Config
	defaultRegion     string
	instanceTypeInfos map[string]instanceTypeInfos
	cache             *cache.Memory
}

func NewEC2VolumeEstimator(awscfg aws.Config, defaultRegion string) *EC2VolumeEstimator {
	return &EC2VolumeEstimator{
		awscfg:            awscfg,
		defaultRegion:     defaultRegion,
		instanceTypeInfos: make(map[string]instanceTypeInfos),
		cache:             cache.NewMemory(5 * time.Minute),
	}
}

func (ec2explorer *EC2VolumeEstimator) collectMetrics(ctx context.Context, region string, metrics chan *cloudcarbonexporter.Metric) error {
	if region == "global" {
		return nil
	}

	ec2api := ec2.NewFromConfig(ec2explorer.awscfg, func(o *ec2.Options) {
		o.Region = region
	})

	paginator := ec2.NewDescribeVolumesPaginator(ec2api, &ec2.DescribeVolumesInput{
		MaxResults: aws.Int32(100),
	})

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return &cloudcarbonexporter.ExplorerErr{Err: fmt.Errorf("failed to list region ec2 volumes: %w", err), Operation: "service/ec2:DescribeVolumes"}
		}

		for _, volume := range output.Volumes {
			watts := 0.0
			if isVolumeHDD(volume.VolumeType) {
				watts = cloud.EstimateHDDBlockStorage(float64(*volume.Size))
			}
			if isVolumeSSD(volume.VolumeType) {
				watts = cloud.EstimateSSDBlockStorage(float64(*volume.Size))
			}

			metrics <- &cloudcarbonexporter.Metric{
				Name: "estimated_watts",
				Labels: cloudcarbonexporter.MergeLabels(
					map[string]string{
						"region":      region,
						"az":          *volume.AvailabilityZone,
						"kind":        "ec2/volume",
						"volume_id":   *volume.VolumeId,
						"volume_type": string(volume.VolumeType),
					},
					parseEC2Tags(volume.Tags),
				),
				Value: watts,
			}
		}
	}

	return nil
}

func (rc *EC2VolumeEstimator) load(ctx context.Context) error { return nil }

func isVolumeHDD(volumeType types.VolumeType) bool {
	switch volumeType {
	case types.VolumeTypeStandard:
		return true
	case types.VolumeTypeSc1:
		return true
	case types.VolumeTypeSt1:
		return true
	default:
		return false
	}
}

func isVolumeSSD(volumeType types.VolumeType) bool {
	return !isVolumeHDD(volumeType)
}
