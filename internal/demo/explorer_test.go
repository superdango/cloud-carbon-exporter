package demo_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
	"github.com/superdango/cloud-carbon-exporter/internal/demo"
)

func TestDemoExplorer(t *testing.T) {
	metrics := make(chan *cloudcarbonexporter.Metric, 1)
	errs := make(chan error)
	demoExplorer := demo.NewExplorer()
	demoExplorer.CollectMetrics(t.Context(), metrics, errs)
	m := <-metrics
	assert.Equal(t, "demo_connected_users", m.Name)
}
