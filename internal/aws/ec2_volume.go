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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
	"github.com/superdango/cloud-carbon-exporter/model/cloud"
)

type EC2VolumeExplorer struct {
	*Explorer
}

func NewEC2VolumeExplorer(explorer *Explorer) *EC2VolumeExplorer {
	return &EC2VolumeExplorer{
		Explorer: explorer,
	}
}

func (ec2explorer *EC2VolumeExplorer) support() string {
	return "ec2/volume"
}

func (ec2explorer *EC2VolumeExplorer) collectImpacts(ctx cloudcarbonexporter.Context, region string, impacts chan *cloudcarbonexporter.Impact) error {
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
		ctx.IncrCalls()
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return &cloudcarbonexporter.ExplorerErr{Err: fmt.Errorf("failed to list region ec2 volumes: %w", err), Operation: "service/ec2:DescribeVolumes"}
		}

		for _, volume := range output.Volumes {
			var energy cloudcarbonexporter.Energy
			var embodiedEmission cloudcarbonexporter.EmissionsOverTime
			if isVolumeHDD(string(volume.VolumeType)) {
				energy = cloud.EstimateHDDBlockStorageEnergy(float64(*volume.Size))
				embodiedEmission = cloud.EstimateHDDBlockStorageEmbodiedEmissions(float64(*volume.Size))
			}
			if isVolumeSSD(string(volume.VolumeType)) {
				energy = cloud.EstimateSSDBlockStorageEnergy(float64(*volume.Size))
				embodiedEmission = cloud.EstimateSSDBlockStorageEmbodiedEmissions(float64(*volume.Size))
			}

			impacts <- &cloudcarbonexporter.Impact{
				Energy:            energy,
				EmbodiedEmissions: embodiedEmission,
				Labels: cloudcarbonexporter.MergeLabels(
					map[string]string{
						"location":    region,
						"az":          *volume.AvailabilityZone,
						"kind":        "ec2/volume",
						"volume_id":   *volume.VolumeId,
						"volume_type": string(volume.VolumeType),
					},
					parseEC2Tags(volume.Tags),
				),
			}
		}
	}

	return nil
}

func (rc *EC2VolumeExplorer) load(ctx context.Context) error { return nil }

func isVolumeHDD(volumeType string) bool {
	switch volumeType {
	case "standard":
		return true
	case "sc1":
		return true
	case "st1":
		return true
	default:
		return false
	}
}

func isVolumeSSD(volumeType string) bool {
	return !isVolumeHDD(volumeType)
}
