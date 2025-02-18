package gcp

import (
	"github.com/superdango/cloud-carbon-exporter"
	"context"
	"fmt"
	"log/slog"
	"time"

	"golang.org/x/sync/errgroup"
)

// Collector implements the cloudcarbon collector interface. It compute metrics by listing
// resources from Google Cloud Assets Inventory then getting their signals from Cloud
// Monitoring to finely compute the realtime energy draw
type Collector struct {
	inventory  *assetInventory
	monitoring *monitoringService
}

// NewCollector returns a new Collector
func NewCollector(ctx context.Context, projectID string) (*Collector, error) {
	inventory, err := newInventoryService(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to create gcp inventory service for project id: %s: %w", projectID, err)
	}

	monitoring, err := newMonitoringService(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to create gcp monitoring service for project id: %s: %w", projectID, err)
	}

	return &Collector{
		inventory:  inventory,
		monitoring: monitoring,
	}, nil
}

// Collect metrics from Google APIs and estimate energy draw of all resources found
func (collector *Collector) Collect(ctx context.Context, ch chan cloudcarbonexporter.Metric) error {
	models := getModels()

	resources, err := collector.inventory.CollectResources(ctx, ch)
	if err != nil {
		return fmt.Errorf("failed to list gcp inventory resources: %w", err)
	}

	errg, errgctx := errgroup.WithContext(ctx)
	for _, resourceKind := range resources.DiscoveredKinds() {
		for metricID, query := range models.getResourceQueries(resourceKind) {
			resourceKind := resourceKind
			query := query
			errg.Go(func() error {
				start := time.Now()
				metrics, err := collector.monitoring.query(errgctx, query, models.getResourceNameField(resourceKind, metricID))
				if err != nil {
					return fmt.Errorf("failed to query monitoring resource kind: %s: %w", resourceKind, err)
				}
				for _, metric := range metrics {
					resource, found := resources.Find(resourceKind, metric.ResourceID)
					if !found {
						resource.Location = "global"
						resource.Labels = nil
					}
					ch <- models.wattMetricFunc(resourceKind, metricID)(cloudcarbonexporter.Metric{
						Name:  "estimated_watts",
						Value: metric.Value,
						Labels: cloudcarbonexporter.MergeLabels(metric.Labels, resource.Labels, map[string]string{
							"model_version": getModelsVersion(),
							"location":      resource.Location,
							"resource_kind": resourceKind,
							"metric_id":     metricID,
						}),
					})
					slog.Debug("metric sent over channel", "name", metric.Name, "labels", metric.Labels, "resource_name", resource.ID)
				}
				slog.Debug("sent gcp monitoring query range", "query", query, "duration_ms", time.Since(start).Milliseconds())
				return nil
			})
		}
	}

	defer close(ch)

	return errg.Wait()
}

// Close collector
func (collector *Collector) Close() error {
	return collector.inventory.Close()
}
