package gcp

import (
	"context"
	"fmt"

	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"

	asset "cloud.google.com/go/asset/apiv1"
	"cloud.google.com/go/asset/apiv1/assetpb"
	"google.golang.org/api/iterator"
)

type Option func(e *Explorer)

type Explorer struct {
	assetClient *asset.Client
	projectID   string
}

func WithProjectID(projectID string) Option {
	return func(e *Explorer) {
		e.projectID = projectID
	}
}

func NewExplorer(ctx context.Context, opts ...Option) (*Explorer, error) {
	var err error
	explorer := new(Explorer)

	for _, c := range opts {
		c(explorer)
	}

	explorer.assetClient, err = asset.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create google cloud explorer: %w", err)
	}
	return explorer, nil
}

func (explorer *Explorer) Find(ctx context.Context, resources chan *cloudcarbonexporter.Resource) error {
	req := &assetpb.ListAssetsRequest{
		Parent:      fmt.Sprintf("projects/%s", explorer.projectID),
		ContentType: assetpb.ContentType_RESOURCE,
	}

	it := explorer.assetClient.ListAssets(ctx, req)
	for {
		response, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to iterate on the next asset: %w", err)
		}

		r := &cloudcarbonexporter.Resource{
			CloudProvider: "gcp",
			Kind:          response.AssetType,
			ID:            response.Resource.Data.GetFields()["name"].GetStringValue(),
			Region:        response.Resource.Location,
			Labels:        mapToStringMap(response.Resource.Data.AsMap()["labels"]),
			Source:        response.Resource.Data.AsMap(),
		}

		resources <- r
	}

	return nil
}

func (explorer *Explorer) Close() error {
	return explorer.assetClient.Close()
}

func (explorer *Explorer) IsReady() bool {
	return explorer.assetClient != nil
}

func mapToStringMap(m any) map[string]string {
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
