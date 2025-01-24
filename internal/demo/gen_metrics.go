package demo

import (
	"cloudcarbonexporter"
	"context"
	"math/rand/v2"
	"time"
)

type DemoCollector struct{}

func NewCollector() *DemoCollector {
	return &DemoCollector{}
}

func (collector *DemoCollector) Collect(ctx context.Context, ch chan cloudcarbonexporter.Metric) error {

	ch <- cloudcarbonexporter.Metric{
		Name:  "demo_connected_users",
		Value: float64(factor()),
		Labels: map[string]string{
			"app": "demo.carbondriven.dev",
		},
	}
	return nil
}

func (collector *DemoCollector) Close() error {
	return nil
}

func factor() int {
	hour := time.Now().Hour()
	factor := []int{2, 1, 2, 1, 1, 2, 3, 4, 5, 6, 5, 6, 5, 4, 3, 4, 5, 6, 5, 6, 7, 6, 5, 4, 3}

	if hour < 23 && factor[hour] > factor[hour+1] {
		return factor[time.Now().Hour()]*100 + time.Now().Minute()*100/60 + rand.Int()%6

	}

	if hour < 23 && factor[hour] == factor[hour+1] {
		return factor[time.Now().Hour()]*100  + rand.Int()%10

	}

	return factor[time.Now().Hour()]*100 - (60-time.Now().Minute())*100/60 + rand.Int()%6

}
