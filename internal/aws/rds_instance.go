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
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/rds/types"
	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
	"github.com/superdango/cloud-carbon-exporter/internal/must"
	"github.com/superdango/cloud-carbon-exporter/model/cloud"
	"github.com/superdango/cloud-carbon-exporter/model/primitives"
)

type RDSInstanceExplorer struct {
	*Explorer
}

func NewRDSInstanceExplorer(explorer *Explorer) *RDSInstanceExplorer {
	return &RDSInstanceExplorer{
		Explorer: explorer,
	}
}

func (rdsExplorer *RDSInstanceExplorer) support() string {
	return "rds/instance"
}

func (rdsExplorer *RDSInstanceExplorer) collectMetrics(ctx context.Context, region string, metrics chan *cloudcarbonexporter.Metric) error {
	if region == "global" {
		return nil
	}

	rdsApi := rds.NewFromConfig(rdsExplorer.awscfg, func(o *rds.Options) {
		o.Region = region
	})

	paginator := rds.NewDescribeDBInstancesPaginator(rdsApi, &rds.DescribeDBInstancesInput{
		MaxRecords: aws.Int32(100),
	})

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return &cloudcarbonexporter.ExplorerErr{Err: fmt.Errorf("failed to list region rds instance: %w", err), Operation: "service/rds:DescribeDBInstances"}
		}

		for _, instance := range output.DBInstances {
			watts := 0.0

			instanceID := *instance.DBInstanceIdentifier
			instanceType := strings.TrimPrefix(*instance.DBInstanceClass, "db.")

			switch instanceType {
			case "serverless":
				serverlessWatts, err := rdsExplorer.serverlessInstanceToWatts(ctx, region, instance)
				if err != nil {
					return fmt.Errorf("failed to get watt for serverless rds instance '%s': %w", instanceID, err)
				}
				watts += serverlessWatts
			default:
				classicInstanceWatt, err := rdsExplorer.classicInstanceToWatts(ctx, region, instance, instanceType)
				if err != nil {
					return fmt.Errorf("failed to get watt for classic rds instance '%s': %w", instanceID, err)
				}
				watts += classicInstanceWatt
			}

			storageWatts := cloud.EstimateSSDBlockStorageWatts(float64(*instance.AllocatedStorage))
			if isVolumeHDD(*instance.StorageType) {
				storageWatts = cloud.EstimateHDDBlockStorageWatts(float64(*instance.AllocatedStorage))
			}
			watts += storageWatts

			metrics <- &cloudcarbonexporter.Metric{
				Name: "estimated_watts",
				Labels: cloudcarbonexporter.MergeLabels(
					map[string]string{
						"kind":        "rds/db_instance",
						"location":    rdsExplorer.Region(*instance.AvailabilityZone),
						"az":          *instance.AvailabilityZone,
						"instance_id": *instance.DBInstanceIdentifier,
					},
					parseRDSTagList(instance.TagList),
				),
				Value: watts,
			}
		}
	}

	return nil
}

// classicInstanceToWatts estimates watts for classic instance using machine type and CPU usage
func (rdsExplorer *RDSInstanceExplorer) classicInstanceToWatts(ctx context.Context, region string, instance types.DBInstance, instanceType string) (float64, error) {

	instanceInfos, found := rdsExplorer.instanceTypeInfos[instanceType]
	if !found {
		return 0.0, fmt.Errorf("rds instance infos not found for type: %s", instanceType)
	}
	cpuAverage, err := rdsExplorer.GetInstanceCPUAverage(ctx, region, *instance.DBInstanceIdentifier)
	if err != nil {
		return 0.0, fmt.Errorf("failed to get rds instance cpu average: %w", err)
	}
	watts := primitives.EstimateMemoryWatts(instanceInfos.Memory)
	watts += primitives.
		LookupProcessorByName(instanceInfos.PhysicalProcessor).
		EstimateCPUWatts(instanceInfos.VCPU, cpuAverage)

	return watts, err
}

// serverlessInstanceToWatts estimates watts for serverless instance using ACUs
func (rdsExplorer *RDSInstanceExplorer) serverlessInstanceToWatts(ctx context.Context, region string, instance types.DBInstance) (float64, error) {
	watts := 0.0
	acuAverage, err := rdsExplorer.GetInstanceACUAverage(ctx, region, *instance.DBInstanceIdentifier)
	if err != nil {
		return 0.0, fmt.Errorf("failed to get rds instance cpu average: %w", err)
	}

	noACU := acuAverage == 0.0
	if noACU {
		return 0.0, nil
	}

	cpuThreadsByACU := 0.5
	memoryByACU := 2.0
	threads := acuAverage * cpuThreadsByACU

	watts += primitives.EstimateMemoryWatts(acuAverage * memoryByACU)
	watts += primitives.LookupProcessorByName("Graviton4").EstimateCPUWatts(threads, 60)

	return watts, err
}

func (rc *RDSInstanceExplorer) load(ctx context.Context) error { return nil }

func (rdsExplorer *RDSInstanceExplorer) GetInstanceCPUAverage(ctx context.Context, region string, instanceID string) (float64, error) {
	key := fmt.Sprintf("%s/rds_instances_average_cpu", region)

	rdsExplorer.cache.SetDynamicIfNotExists(ctx, key, func(ctx context.Context) (any, error) {
		return rdsExplorer.ListInstanceCPUAverage(ctx, region)
	}, 5*time.Minute)

	entry, err := rdsExplorer.cache.Get(ctx, key)
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
func (ec2explorer *RDSInstanceExplorer) ListInstanceCPUAverage(ctx context.Context, region string) (map[string]float64, error) {
	metricName := "cpu_utilization_by_instance_id"
	cloudwatchExpression := `SELECT AVG(CPUUtilization) FROM "AWS/RDS" GROUP BY DBInstanceIdentifier`
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
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, &cloudcarbonexporter.ExplorerErr{
				Operation: "cloudwatch:GetMetricData",
				Err:       fmt.Errorf("failed to get rds instances cloudwatch metric in region %s: %w", region, err),
			}

		}

		for _, metricData := range page.MetricDataResults {
			instanceList[region+"/"+*metricData.Label] = metricData.Values[0]
		}

	}
	return instanceList, nil
}

func (rdsExplorer *RDSInstanceExplorer) GetInstanceACUAverage(ctx context.Context, region string, instanceID string) (float64, error) {
	key := fmt.Sprintf("%s/rds_serverless_instances_average_acu", region)

	rdsExplorer.cache.SetDynamicIfNotExists(ctx, key, func(ctx context.Context) (any, error) {
		return rdsExplorer.ListInstanceACUAverage(ctx, region)
	}, 5*time.Minute)

	entry, err := rdsExplorer.cache.Get(ctx, key)
	if err != nil {
		return 0.0, fmt.Errorf("failed to list serverless instance acu average: %w", err)
	}

	instancesAverageACU, ok := entry.(map[string]float64)
	must.Assert(ok, "instancesAverageACU is not a map[string]float64")

	instanceAverageACU, found := instancesAverageACU[region+"/"+instanceID]
	if !found {
		return 1.0, nil // minimum cpu average
	}

	return instanceAverageACU, nil
}

// ListInstanceCPUAverage returns the 10 minutes average ACU for all serverless instances in the region
func (ec2explorer *RDSInstanceExplorer) ListInstanceACUAverage(ctx context.Context, region string) (map[string]float64, error) {
	metricName := "acu_utilization_by_instance_id"
	cloudwatchExpression := `SELECT AVG(ACUUtilization) FROM "AWS/RDS" GROUP BY DBInstanceIdentifier`
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
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, &cloudcarbonexporter.ExplorerErr{
				Operation: "cloudwatch:GetMetricData",
				Err:       fmt.Errorf("failed to get rds instances cloudwatch metric in region %s: %w", region, err),
			}

		}

		for _, metricData := range page.MetricDataResults {
			instanceList[region+"/"+*metricData.Label] = metricData.Values[0]
		}

	}

	return instanceList, nil
}

func parseRDSTagList(list []types.Tag) map[string]string {
	labels := make(map[string]string)
	for _, t := range list {
		labels[*t.Key] = "tag_" + *t.Value
	}
	return labels
}
