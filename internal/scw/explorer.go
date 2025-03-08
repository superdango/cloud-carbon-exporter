package scw

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/scaleway/scaleway-sdk-go/api/instance/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
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
	client  *scw.Client
	regions []scw.Region
}

func NewExplorer(opts ...ExplorerOption) (*Explorer, error) {
	e := &Explorer{
		regions: scw.AllRegions,
	}

	for _, opt := range opts {
		opt(e)
	}

	if e.client == nil {
		return nil, fmt.Errorf("scaleway client is required")
	}

	return e, nil
}

func (e *Explorer) Find(ctx context.Context, resources chan *cloudcarbonexporter.Resource, errs chan error) {
	wg := new(sync.WaitGroup)
	for _, region := range e.regions {
		region := region
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- e.findRegionalInstances(ctx, region, resources)
		}()
	}
	wg.Wait()

}

func (e *Explorer) IsReady() bool { return true }

func (e *Explorer) Close() error { return nil }

func (e *Explorer) findRegionalInstances(ctx context.Context, region scw.Region, resources chan *cloudcarbonexporter.Resource) error {
	api := instance.NewAPI(e.client)

	resp, err := api.ListServers(&instance.ListServersRequest{Zone: scw.ZonePlWaw1}, scw.WithContext(ctx), scw.WithAllPages(), scw.WithZones(region.GetZones()...))
	if err != nil {
		return &cloudcarbonexporter.ExplorerErr{Err: fmt.Errorf("failed to list %s region servers: %w", region, err), Operation: "instance/v1:ListServers"}
	}

	for _, server := range resp.Servers {
		resources <- &cloudcarbonexporter.Resource{
			CloudProvider: "scw",
			Kind:          "instance",
			ID:            server.ID,
			Region:        region.String(),
			Labels: map[string]string{
				"name":          server.Name,
				"project":       server.Project,
				"instance_type": server.CommercialType,
				"tags":          strings.Join(server.Tags, ","),
			},
			Source: cloudcarbonexporter.AnyMap{
				"server": server,
			},
		}
	}

	return nil

}
