package scw

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/scaleway/scaleway-sdk-go/api/instance/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
	"github.com/superdango/cloud-carbon-exporter/model/carbon"
	"github.com/superdango/cloud-carbon-exporter/model/energy/primitives"
)

type ExplorerOption func(*Explorer)

// WithClient sets scaleway client for the explorer
func WithClient(client *scw.Client) ExplorerOption {
	return func(e *Explorer) {
		e.client = client
	}
}

// WithRegions sets regions to explore.
func WithRegions(regions ...string) ExplorerOption {
	return func(c *Explorer) {
		r := make([]scw.Region, len(regions))
		for i, region := range regions {
			r[i] = scw.Region(region)
		}
		c.regions = r
	}
}

type Explorer struct {
	client             *scw.Client
	regions            []scw.Region
	carbonIntensityMap carbon.IntensityMap
}

func NewExplorer(opts ...ExplorerOption) (*Explorer, error) {
	e := &Explorer{
		regions:            scw.AllRegions,
		carbonIntensityMap: carbon.NewScalewayCloudCarbonFootprintIntensityMap(),
	}

	for _, opt := range opts {
		opt(e)
	}

	if e.client == nil {
		return nil, fmt.Errorf("scaleway client is required")
	}

	return e, nil
}

func (explorer *Explorer) CollectMetrics(ctx context.Context, metrics chan *cloudcarbonexporter.Metric, errs chan error) {
	wg := new(sync.WaitGroup)

	energyMetrics := make(chan *cloudcarbonexporter.Metric)
	defer close(energyMetrics)

	go func() {
		for energyMetric := range energyMetrics {
			metrics <- energyMetric
			metrics <- explorer.carbonIntensityMap.ComputeCO2eq(energyMetric)
		}
	}()

	for _, region := range explorer.regions {
		region := region
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- explorer.findRegionalInstances(ctx, region, energyMetrics)
		}()
	}
	wg.Wait()

}

func (explorer *Explorer) IsReady() bool { return true }

func (explorer *Explorer) Close() error { return nil }

func (explorer *Explorer) findRegionalInstances(ctx context.Context, region scw.Region, metrics chan *cloudcarbonexporter.Metric) error {
	api := instance.NewAPI(explorer.client)

	resp, err := api.ListServers(&instance.ListServersRequest{Zone: scw.ZonePlWaw1}, scw.WithContext(ctx), scw.WithAllPages(), scw.WithZones(region.GetZones()...))
	if err != nil {
		return &cloudcarbonexporter.ExplorerErr{Err: fmt.Errorf("failed to list %s region servers: %w", region, err), Operation: "instance/v1:ListServers"}
	}

	for _, server := range resp.Servers {
		processor := primitives.LookupProcessorByName("TODO")
		watts := processor.EstimateCPUWatts(1, 0)
		watts += primitives.EstimateMemoryWatts(4)

		metrics <- &cloudcarbonexporter.Metric{
			Name: "estimated_watts",
			Labels: map[string]string{
				"name":          server.Name,
				"region":        string(region),
				"project":       server.Project,
				"instance_name": server.Name,
				"tags":          strings.Join(server.Tags, ","),
			},
			Value: watts,
		}
	}

	return nil

}
