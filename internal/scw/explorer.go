package scw

import (
	"context"
	"fmt"
	"strings"

	"github.com/scaleway/scaleway-sdk-go/api/instance/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
	"golang.org/x/sync/errgroup"
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

func (e *Explorer) Find(ctx context.Context, resources chan *cloudcarbonexporter.Resource) error {
	errg, errgctx := errgroup.WithContext(ctx)
	for _, region := range e.regions {
		region := region
		errg.Go(func() error {
			return e.findRegionalInstances(errgctx, region, resources)
		})
	}

	if err := errg.Wait(); err != nil {
		return fmt.Errorf("failed to find resources: %w", err)
	}

	return nil
}

func (e *Explorer) IsReady() bool { return true }

func (e *Explorer) Close() error { return nil }

func (e *Explorer) findRegionalInstances(ctx context.Context, region scw.Region, resources chan *cloudcarbonexporter.Resource) error {
	api := instance.NewAPI(e.client)

	resp, err := api.ListServers(&instance.ListServersRequest{}, scw.WithAllPages(), scw.WithZones(region.GetZones()...))
	if err != nil {
		return fmt.Errorf("failed to list %s region servers: %w", region, err)
	}

	for _, server := range resp.Servers {
		resources <- &cloudcarbonexporter.Resource{
			CloudProvider: "scw",
			Kind:          "instance",
			ID:            server.ID,
			Region:        region.String(),
			Labels: map[string]string{
				"project": server.Project,
				"tags":    strings.Join(server.Tags, ","),
			},
			Source: cloudcarbonexporter.AnyMap{
				"server": server,
			},
		}
	}

	return nil

}
