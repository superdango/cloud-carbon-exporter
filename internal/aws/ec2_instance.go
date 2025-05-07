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
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
	"github.com/superdango/cloud-carbon-exporter/internal/must"
	"github.com/superdango/cloud-carbon-exporter/model/primitives"
)

//go:embed data/instance_types/instance_types.json
var instanceTypeJsonFile embed.FS

type instanceTypeInfos struct {
	InstanceType      string  `json:"instance_type"`
	PhysicalProcessor string  `json:"physical_processor"`
	VCPU              float64 `json:"vcpu"`
	Memory            float64 `json:"memory"`
	GPU               float64 `json:"gpu"`
	GPUMemory         float64 `json:"gpu_memory"`
	SSDCount          float64 `json:"ssd_count"`
	SSDSize           float64 `json:"ssd_size"`
	HDDCount          float64 `json:"hdd_count"`
	HDDSize           float64 `json:"hdd_size"`
}

type EC2InstanceExplorer struct {
	*Explorer
}

func NewEC2InstanceExplorer(explorer *Explorer) *EC2InstanceExplorer {
	return &EC2InstanceExplorer{
		Explorer: explorer,
	}
}

func (ec2explorer *EC2InstanceExplorer) support() string {
	return "ec2/instance"
}

func (ec2explorer *EC2InstanceExplorer) load(ctx context.Context) error {
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

	ec2explorer.mu.Lock()
	defer ec2explorer.mu.Unlock()

	for _, infos := range instancesTypeInfos {
		ec2explorer.instanceTypeInfos[infos.InstanceType] = infos
	}

	slog.Info("ec2 instance types infos loaded")

	return nil
}

func (ec2explorer *EC2InstanceExplorer) collectImpacts(ctx cloudcarbonexporter.Context, region string, impacts chan *cloudcarbonexporter.Impact) error {
	if region == "global" {
		return nil
	}

	ec2api := ec2.NewFromConfig(ec2explorer.awscfg, func(o *ec2.Options) {
		o.Region = region
	})

	paginator := ec2.NewDescribeInstancesPaginator(ec2api, &ec2.DescribeInstancesInput{
		MaxResults: aws.Int32(100),
	})

	for paginator.HasMorePages() {
		ctx.IncrCalls()
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return &cloudcarbonexporter.ExplorerErr{Err: fmt.Errorf("failed to list region ec2 instances: %w", err), Operation: "service/ec2:DescribeInstances"}
		}

		for _, reservation := range output.Reservations {
			for _, instance := range reservation.Instances {
				if instance.State.Name != types.InstanceStateNameRunning {
					continue
				}
				instanceType := ec2explorer.instanceTypeInfos[string(instance.InstanceType)]
				intanceAverageCPU, err := ec2explorer.GetInstanceCPUAverage(ctx, region, *instance.InstanceId)
				if err != nil {
					return fmt.Errorf("failed to get instance %s cpu average: %w", *instance.InstanceId, err)
				}

				processor := primitives.LookupProcessorByName(instanceType.PhysicalProcessor)
				energy := processor.EstimateCPUEnergy(instanceType.VCPU, intanceAverageCPU)
				energy += primitives.EstimateMemoryEnergy(instanceType.Memory)
				cpuEmbodied := primitives.EstimateCPUEmbodiedEmissions(instanceType.VCPU)
				memoryEmbodied := primitives.EstimateMemoryEmbodiedEmissions(instanceType.Memory)

				diskEmbodied := cloudcarbonexporter.ZeroEmissions
				if instanceType.SSDCount > 0 {
					diskEmbodied = cloudcarbonexporter.CombineEmissionsOverTime(diskEmbodied, primitives.EstimateEmbodiedSSDEmissions(instanceType.SSDSize))
				}
				if instanceType.HDDCount > 0 {
					diskEmbodied = cloudcarbonexporter.CombineEmissionsOverTime(diskEmbodied, primitives.EstimateEmbodiedHDDEmissions(instanceType.HDDCount))
				}

				impacts <- &cloudcarbonexporter.Impact{
					Energy:            energy,
					EmbodiedEmissions: cloudcarbonexporter.CombineEmissionsOverTime(cpuEmbodied, memoryEmbodied, diskEmbodied),
					Labels: cloudcarbonexporter.MergeLabels(
						parseEC2Tags(instance.Tags),
						map[string]string{
							"location":    region,
							"az":          *instance.Placement.AvailabilityZone,
							"kind":        "ec2/instance",
							"instance_id": *instance.InstanceId,
						},
					),
				}
			}
		}
	}

	return nil
}

func (ec2explorer *EC2InstanceExplorer) GetInstanceCPUAverage(ctx cloudcarbonexporter.Context, region string, instanceID string) (float64, error) {
	key := fmt.Sprintf("%s/instances_average_cpu", region)

	ec2explorer.cache.SetDynamicIfNotExists(ctx, key, func(ctx context.Context) (any, error) {
		return ec2explorer.ListInstanceCPUAverage(cloudcarbonexporter.WrapCtx(ctx), region)
	}, 5*time.Minute)

	entry, err := ec2explorer.cache.Get(ctx, key)
	if err != nil {
		return 0.0, fmt.Errorf("failed to list instance cpu average: %w", err)
	}

	instancesAverageCPU, ok := entry.(map[string]float64)
	must.Assert(ok, "instancesAverageCPU is not a map[string]float64")

	instanceAverageCPU, found := instancesAverageCPU[region+"/"+instanceID]
	if !found {
		return 1.0, nil // minimum cpu average
	}

	return instanceAverageCPU, nil
}

// ListInstanceCPUAverage returns the 10 minutes average cpu for all instances in the region
func (ec2explorer *EC2InstanceExplorer) ListInstanceCPUAverage(ctx cloudcarbonexporter.Context, region string) (map[string]float64, error) {
	metricName := "cpu_utilization_by_instance_id"
	cloudwatchExpression := `SELECT AVG(CPUUtilization) FROM "AWS/EC2" GROUP BY InstanceId`
	period := 10 * time.Minute
	instanceList := make(map[string]float64)

	cwapi := cloudwatch.NewFromConfig(ec2explorer.awscfg, func(o *cloudwatch.Options) {
		o.Region = region
	})

	paginator := cloudwatch.NewGetMetricDataPaginator(cwapi, &cloudwatch.GetMetricDataInput{
		// TODO: For better performance, specify StartTime and EndTime values that align with the value of the metric's Period
		StartTime: aws.Time(time.Now().Add(-period)),
		EndTime:   aws.Time(time.Now()),
		MetricDataQueries: []cwtypes.MetricDataQuery{
			{
				Id:         aws.String(metricName),
				Expression: aws.String(cloudwatchExpression),
				Period:     aws.Int32(int32(period.Seconds())),
			},
		},
	})

	for paginator.HasMorePages() {
		ctx.IncrCalls()
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, &cloudcarbonexporter.ExplorerErr{
				Operation: "cloudwatch:GetMetricData",
				Err:       fmt.Errorf("failed to get instances cloudwatch metric in region %s: %w", region, err),
			}

		}

		for _, metricData := range page.MetricDataResults {
			instanceList[region+"/"+*metricData.Label] = metricData.Values[0]
		}

	}
	return instanceList, nil
}

func parseEC2Tags(tags []types.Tag) map[string]string {
	m := make(map[string]string, len(tags))

	for _, t := range tags {
		m[fmt.Sprintf("tag_%s", *t.Key)] = *t.Value
	}

	return m
}
