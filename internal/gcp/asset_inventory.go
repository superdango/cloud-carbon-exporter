package gcp

import (
	"cloudcarbonexporter"
	"context"
	"fmt"
	"log/slog"

	asset "cloud.google.com/go/asset/apiv1"
	"cloud.google.com/go/asset/apiv1/assetpb"
	"google.golang.org/api/iterator"
)

type assetInventory struct {
	client    *asset.Client
	projectID string
}

func newInventoryService(ctx context.Context, projectID string) (*assetInventory, error) {
	var err error
	inventory := &assetInventory{
		projectID: projectID,
	}
	inventory.client, err = asset.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCP inventory: %w", err)
	}
	return inventory, nil
}

func (inventory *assetInventory) Close() error {
	return inventory.client.Close()
}

func (inventory *assetInventory) List(ctx context.Context) (cloudcarbonexporter.Resources, error) {
	models := getModels()
	req := &assetpb.ListAssetsRequest{
		Parent: fmt.Sprintf("projects/%s", inventory.projectID),
		// AssetTypes:  SUPPORTED_RESOURCES_LIST(), // TODO: try to filter via the API
		// No RESOURCE found that matches asset type for consumer project
		ContentType: assetpb.ContentType_RESOURCE,
	}
	resources := []cloudcarbonexporter.Resource{
		{
			Kind: "serviceruntime.googleapis.com/api",
		},
	}
	it := inventory.client.ListAssets(ctx, req)
	for {
		response, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate on the next asset: %w", err)
		}

		// TODO: filter ressource directly in the api call
		if !models.isSupportedRessource(response.AssetType) {
			slog.Debug("resource not supported", "kind", response.AssetType)
			continue
		}

		r := cloudcarbonexporter.Resource{
			Kind:     response.AssetType,
			Name:     response.Resource.Data.GetFields()["name"].GetStringValue(),
			Location: response.Resource.Location,
			Labels:   toStringMap(response.Resource.Data.AsMap()["labels"]),
			Source:   response.Resource.Data.AsMap(),
		}

		if metricFunc, found := getModels().assets[r.Kind]; found {
			slog.Debug("resource from assets is sufficient for metric calculation", "kind", r.Kind)
			r.Metric = metricFunc(r)
		}

		resources = append(resources, r)
	}

	return resources, nil
}

func toStringMap(m any) map[string]string {
	mapOfAny, ok := m.(map[string]any)
	if !ok {
		return make(map[string]string)
	}

	mapOfString := make(map[string]string)
	for k, v := range mapOfAny {
		mapOfString[k] = fmt.Sprintf("%s", v)
	}

	return mapOfString
}
