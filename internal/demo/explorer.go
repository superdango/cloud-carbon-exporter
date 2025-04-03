package demo

import (
	"context"
	"math/rand/v2"
	"time"

	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"
)

// Explorer implements the cloudcarbon collector interface.
// It is used to generate fake data for demonstration purpose
type Explorer struct{}

// NewExplorer returns a new demo collector
func NewExplorer() *Explorer {
	return &Explorer{}
}

func (explorer *Explorer) Close() error {
	return nil
}

func (explorer *Explorer) IsReady() bool {
	return true
}

func (explorer *Explorer) CollectMetrics(ctx context.Context, metrics chan *cloudcarbonexporter.Metric, errs chan error) {
	explorer.generatesDemoMetrics(metrics)
}

func (explorer *Explorer) generatesDemoMetrics(metrics chan *cloudcarbonexporter.Metric) {
	now := time.Now()
	metrics <- &cloudcarbonexporter.Metric{
		Name:  "demo_connected_users",
		Value: float64(naturalTrafficInstant(now.Hour(), now.Minute(), rand.IntN(10))),
		Labels: map[string]string{
			"app": "demo.carbondriven.dev",
		},
	}
}

// naturalTrafficInstant generate a trafic value with hourly variation
func naturalTrafficInstant(hour, minute, rand int) int {
	hourlyTraficCoefficient := map[int]int{
		0:  300,
		1:  200,
		2:  190,
		3:  200,
		4:  190,
		5:  200,
		6:  300,
		7:  400,
		8:  500,
		9:  600,
		10: 500,
		11: 600,
		12: 500,
		13: 400,
		14: 300,
		15: 400,
		16: 500,
		17: 600,
		18: 500,
		19: 600,
		20: 700,
		21: 600,
		22: 500,
		23: 400,
	}

	noise := rand * hourlyTraficCoefficient[hour] / 100

	descending := hourlyTraficCoefficient[hour] > hourlyTraficCoefficient[hour+1]
	if descending {
		return hourlyTraficCoefficient[hour] + (60-minute)*100/60 + noise
	}

	return hourlyTraficCoefficient[hour] + minute*100/60 + noise
}
