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
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
)

//go:embed data/instance_types/instance_types.json
var instanceTypeJsonFile embed.FS

type instanceTypeInfos struct {
	InstanceType      string  `json:"instance_type"`
	PhysicalProcessor string  `json:"physical_processor"`
	VCPU              float64 `json:"vcpu"`
	Memory            float64 `json:"memory"`
}

type EC2InstanceExplorer struct {
	awscfg            aws.Config
	defaultRegion     string
	instanceTypeInfos map[string]instanceTypeInfos
}

func NewEC2InstanceExplorer(awscfg aws.Config, defaultRegion string) *EC2InstanceExplorer {
	return &EC2InstanceExplorer{
		awscfg:            awscfg,
		defaultRegion:     defaultRegion,
		instanceTypeInfos: make(map[string]instanceTypeInfos),
	}
}

func (rc *EC2InstanceExplorer) Load(ctx context.Context) error {
	file, err := instanceTypeJsonFile.Open("data/instance_types/instance_types.json")
	if err != nil {
		return fmt.Errorf("failed to open instance type json file: %w", err)
	}
	defer file.Close()

	instancesTypeInfos := make([]instanceTypeInfos, 0)

	err = json.NewDecoder(file).Decode(&instancesTypeInfos)
	if err != nil {
		return fmt.Errorf("failed to decode instance type json file: %w", err)
	}

	for _, infos := range instancesTypeInfos {
		rc.instanceTypeInfos[infos.InstanceType] = infos
	}

	slog.Info("ec2 instance types infos loaded")

	return nil
}

func (rc *EC2InstanceExplorer) CreateResources(ctx context.Context, region string, resources chan *cloudcarbonexporter.Resource) error {
	slog.Debug("calling ec2 instance explorer create resources", "region", region)
	if region == "global" {
		return nil
	}

	ec2api := ec2.NewFromConfig(rc.awscfg, func(o *ec2.Options) {
		o.Region = region
	})

	paginator := ec2.NewDescribeInstancesPaginator(ec2api, &ec2.DescribeInstancesInput{
		MaxResults: aws.Int32(100),
	})

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list region ec2 instances: %w", err)
		}

		for _, reservation := range output.Reservations {
			for _, instance := range reservation.Instances {
				if instance.State.Name != types.InstanceStateNameRunning {
					continue
				}
				instanceType := rc.instanceTypeInfos[string(instance.InstanceType)]
				resources <- &cloudcarbonexporter.Resource{
					CloudProvider: "aws",
					Kind:          "ec2/instance",
					ID:            *instance.InstanceId,
					Region:        region,
					Labels:        parseEC2Tags(instance.Tags),
					Source: map[string]any{
						"ec2_instance_core_count":         instanceType.VCPU,
						"ec2_instance_physical_processor": instanceType.PhysicalProcessor,
						"ec2_instance_memory_gb":          instanceType.Memory,
					},
				}
			}
		}
	}

	return nil
}

func parseEC2Tags(tags []types.Tag) map[string]string {
	m := make(map[string]string, len(tags))

	for _, t := range tags {
		m[fmt.Sprintf("tag_%s", *t.Key)] = *t.Value
	}

	return m
}
