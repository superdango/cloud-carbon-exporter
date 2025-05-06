package scw

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/scaleway/scaleway-sdk-go/api/instance/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
	"github.com/superdango/cloud-carbon-exporter/model/carbon"
	"github.com/superdango/cloud-carbon-exporter/model/primitives"
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

func NewExplorer() *Explorer {
	return &Explorer{
		regions:            scw.AllRegions,
		carbonIntensityMap: carbon.NewScalewayCloudCarbonFootprintIntensityMap(),
	}
}

func (explorer *Explorer) Configure(opts ...ExplorerOption) *Explorer {
	for _, opt := range opts {
		opt(explorer)
	}
	return explorer
}

func (explorer *Explorer) Init(ctx context.Context) (err error) {
	if explorer.client == nil {
		return fmt.Errorf("scaleway client is required")
	}

	return nil
}

func (explorer *Explorer) SupportedServices() []string {
	return []string{}
}

func (explorer *Explorer) Tags() map[string]string {
	return map[string]string{
		"cloud_provider": "scw",
	}
}

func (explorer *Explorer) CollectImpacts(ctx cloudcarbonexporter.Context, impacts chan *cloudcarbonexporter.Impact, errs chan error) {
	wg := new(sync.WaitGroup)

	rawImpacts := make(chan *cloudcarbonexporter.Impact)
	defer close(rawImpacts)

	go func() {
		for rawImpact := range rawImpacts {
			location, found := rawImpact.Labels["location"]
			if !found {
				slog.Warn("impact location not found, skipping impact. please consider raising a bug.", "labels", rawImpact.Labels)
				continue
			}
			rawImpact.Energy = rawImpact.Energy * primitives.GoodPUE
			rawImpact.EnergyEmissions = explorer.carbonIntensityMap.EnergyEmissions(rawImpact.Energy, location)
			impacts <- rawImpact
		}
	}()

	for _, region := range explorer.regions {
		region := region
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- explorer.findRegionalInstances(ctx, region, rawImpacts)
		}()
	}
	wg.Wait()

}

func (explorer *Explorer) IsReady() bool { return true }

func (explorer *Explorer) Close() error { return nil }

func (explorer *Explorer) findRegionalInstances(ctx context.Context, region scw.Region, impacts chan *cloudcarbonexporter.Impact) error {
	api := instance.NewAPI(explorer.client)

	resp, err := api.ListServers(&instance.ListServersRequest{Zone: scw.ZonePlWaw1}, scw.WithContext(ctx), scw.WithAllPages(), scw.WithZones(region.GetZones()...))
	if err != nil {
		return &cloudcarbonexporter.ExplorerErr{Err: fmt.Errorf("failed to list %s region servers: %w", region, err), Operation: "instance/v1:ListServers"}
	}

	for _, server := range resp.Servers {
		processor := primitives.LookupProcessorByName("TODO")
		energy := processor.EstimateCPUEnergy(1, 0)
		energy += primitives.EstimateMemoryEnergy(4)

		impacts <- &cloudcarbonexporter.Impact{
			Energy: energy,
			Labels: map[string]string{
				"name":          server.Name,
				"region":        string(region),
				"project":       server.Project,
				"instance_name": server.Name,
				"tags":          strings.Join(server.Tags, ","),
			},
		}
	}

	return nil

}
