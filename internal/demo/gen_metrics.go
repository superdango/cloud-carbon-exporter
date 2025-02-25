package demo

import (
	"context"

	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
)

// Explorer implements the cloudcarbon collector interface.
// It is used to generate fake data for demonstration purpose
type Explorer struct{}

// NewExplorer returns a new demo collector
func NewExplorer() *Explorer {
	return &Explorer{}
}

func (explorer *Explorer) Find(ctx context.Context, resources chan *cloudcarbonexporter.Resource) error {
	resources <- &cloudcarbonexporter.Resource{
		CloudProvider: "demo",
		Kind:          "demo",
	}

	return nil
}

// Close demo collector
func (explorer *Explorer) Close() error { return nil }

func (explorer *Explorer) IsReady() bool { return true }
