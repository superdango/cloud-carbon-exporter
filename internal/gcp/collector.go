package gcp

import (
	"cloudcarbonexporter"
	"context"
	"fmt"
	"log/slog"
	"time"

	"golang.org/x/sync/errgroup"
)

type Collector struct {
	inventory  *assetInventory
	monitoring *monitoringService
}

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

func (collector *Collector) Collect(ctx context.Context, ch chan cloudcarbonexporter.Metric) error {
	models := getModels()

	resources, err := collector.inventory.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list gcp inventory resources: %w", err)
	}

	for _, r := range resources {
		if r.Metric != nil {
			ch <- *r.Metric
		}
	}

	errg, errgctx := errgroup.WithContext(ctx)
	for _, resourceKind := range resources.DistinctKinds() {
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
					resource, found := resources.Find(resourceKind, metric.ResourceName)
					if !found {
						resource.Location = "global"
						resource.Labels = nil
					}
					ch <- models.getWattTransformer(resourceKind, metricID)(cloudcarbonexporter.Metric{
						Name:  "estimated_watts",
						Value: metric.Value,
						Labels: mergeLabels(metric.Labels, resource.Labels, map[string]string{
							"model_version": getModelsVersion(),
							"location":      resource.Location,
							"resource_kind": resourceKind,
							"metric_id":     metricID,
						}),
					})
					slog.Debug("metric sent over channel", "name", metric.Name, "labels", metric.Labels, "resource_name", resource.Name)
				}
				slog.Debug("sent gcp monitoring query range", "query", query, "duration_ms", time.Since(start).Milliseconds())
				return nil
			})
		}
	}

	defer close(ch)

	return errg.Wait()
}

func (collector *Collector) Close() error {
	return collector.inventory.Close()
}

func mergeLabels(labels ...map[string]string) map[string]string {
	result := make(map[string]string)
	for _, l := range labels {
		for k, v := range l {
			if v == "" {
				continue
			}
			result[k] = v
		}
	}
	return result
}
