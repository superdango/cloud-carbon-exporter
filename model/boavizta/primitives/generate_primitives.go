package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"
)

type BoaviztaAPI struct {
	url    string
	client *http.Client
}

func main() {

	ctx := context.Background()
	apiBoavizta := flag.String("boavizta-api-url", "http://localhost:5000", "url of the boavizta api")
	cloudProvider := flag.String("provider", "scaleway", "provider to export data from")
	flag.Parse()

	api := &BoaviztaAPI{url: *apiBoavizta, client: http.DefaultClient}

	types, err := api.ListCloudInstanceTypes(ctx, *cloudProvider)
	if err != nil {
		panic(err)
	}

	mu := new(sync.Mutex)
	typeIntensity := make(map[string][]JSONFloat)
	errg, errgctx := errgroup.WithContext(ctx)
	for _, t := range types {
		t := t
		loads := make([]JSONFloat, 100)
		errg.Go(func() error {
			for i := range 100 {
				impact, err := api.InstanceImpact(errgctx, *cloudProvider, t, i)
				if err != nil {
					return err
				}
				loads[i] = impact.Verbose.AVGPower.Value
			}
			mu.Lock()
			defer mu.Unlock()
			typeIntensity[t] = loads
			return nil
		})
	}

	if err := errg.Wait(); err != nil {
		panic(err)
	}

	if *cloudProvider == "scaleway" {
		startdustIntensity := make([]JSONFloat, len(typeIntensity["dev1-s"]))
		copy(startdustIntensity, typeIntensity["dev1-s"])
		for i := range len(startdustIntensity) {
			startdustIntensity[i] = startdustIntensity[i] / 2
		}
		typeIntensity["stardust1-s"] = startdustIntensity
	}

	f, err := os.Create(*cloudProvider + ".json")
	if err != nil {
		panic(err)
	}

	err = json.NewEncoder(f).Encode(typeIntensity)
	if err != nil {
		panic(err)
	}
}

func (api *BoaviztaAPI) ListCloudInstanceTypes(ctx context.Context, provider string) ([]string, error) {
	instanceTypes := make([]string, 0)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api.url+"/v1/cloud/instance/all_instances", nil)
	if err != nil {
		return nil, err
	}
	req.URL.RawQuery = "provider=" + provider

	resp, err := api.client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("req: %s, error %d", req.URL.String(), resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(&instanceTypes); err != nil {
		return nil, err
	}

	return instanceTypes, nil
}

type JSONFloat float64

func (pf JSONFloat) MarshalJSON() ([]byte, error) {
	s := fmt.Sprintf("%.2f", pf)
	return []byte(s), nil
}

type InstanceImpactResult struct {
	Impacts struct {
		PE struct {
			Use struct {
				Max   float64 `json:"max"`
				Min   float64 `json:"min"`
				Value float64 `json:"value"`
			} `json:"use"`
		} `json:"pe"`
	} `json:"impacts"`
	Verbose struct {
		AVGPower struct {
			Value JSONFloat `json:"value"`
		} `json:"avg_power"`
	} `json:"verbose"`
}

func (api *BoaviztaAPI) InstanceImpact(ctx context.Context, provider string, instanceType string, percent int) (*InstanceImpactResult, error) {
	payload := map[string]any{
		"instance_type": instanceType,
		"provider":      provider,
		"usage": map[string]any{
			"hours_life_time": 1,
			"time_workload": []any{
				map[string]any{
					"load_percentage": percent,
					"time_percentage": 100,
				},
			},
		},
	}
	jsn, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, api.url+"/v1/cloud/instance", strings.NewReader(string(jsn)))
	if err != nil {
		return nil, err
	}
	req.URL.RawQuery = "verbose=true"

	resp, err := api.client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("req: %s, error %d", req.URL.String(), resp.StatusCode)
	}

	res := new(InstanceImpactResult)
	if err := json.NewDecoder(resp.Body).Decode(res); err != nil {
		return nil, err
	}

	return res, nil
}
