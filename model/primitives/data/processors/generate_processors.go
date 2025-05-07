package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
)

type BoaviztaAPI struct {
	url    string
	client *http.Client
}

func main() {
	ctx := context.Background()
	apiBoavizta := flag.String("boavizta-api-url", "http://localhost:5000", "url of the boavizta api")
	flag.Parse()

	wg := new(sync.WaitGroup)
	for provider := range strings.SplitSeq("aws,azure,gcp,scaleway", ",") {
		wg.Add(1)
		go func() {
			defer wg.Done()
			extractProviderProcessorsTDP(ctx, *apiBoavizta, provider)
		}()
	}
	wg.Wait()
}

func extractProviderProcessorsTDP(ctx context.Context, apiBoavizta string, cloudProvider string) {
	api := &BoaviztaAPI{url: apiBoavizta, client: http.DefaultClient}

	types, err := api.ListCloudInstanceTypes(ctx, cloudProvider)
	if err != nil {
		panic(err)
	}

	f1, err := os.Create(cloudProvider + ".csv")
	if err != nil {
		panic(err)
	}
	defer f1.Close()

	f2, err := os.Create(cloudProvider + "_embodied.csv")
	if err != nil {
		panic(err)
	}
	defer f2.Close()

	w1 := csv.NewWriter(f1)
	defer w1.Flush()
	w1.Comma = ';'
	w1.Write([]string{"name", "family", "tdp", "cores", "threads", "memory", "embodied"})

	w2 := csv.NewWriter(f2)
	defer w2.Flush()
	w2.Comma = ';'
	w2.Write([]string{"name", "vcpu", "memory", "embodied"})

	for _, t := range types {
		impact, err := api.InstanceImpact(ctx, cloudProvider, t, 0)
		if err != nil {
			slog.Warn(err.Error())
			continue
		}

		impact.ArchetypeName = t

		if impact.Verbose.CPU_1.Name.Value == "" ||
			impact.Verbose.CPU_1.Family.Value == "" ||
			impact.Verbose.CPU_1.CoreUnits.Value == 0 {
			continue
		}

		w1.Write([]string{
			impact.Verbose.CPU_1.Name.Value,
			impact.Verbose.CPU_1.Family.Value,
			fmt.Sprintf("%.0f", impact.Verbose.CPU_1.TDP.Value),
			fmt.Sprintf("%d", impact.Verbose.CPU_1.CoreUnits.Value),
			fmt.Sprintf("%.0f", impact.Verbose.CPU_1.Threads.Value),
			fmt.Sprintf("%.1f", impact.Verbose.Memory.Value),
			fmt.Sprintf("%.1f", impact.Impacts.GWP.Embodied.Max),
		})

		if impact.Verbose.SSD_1.Capacity.Value+impact.Verbose.HDD_1.Capacity.Value > 0 {
			slog.Warn("ignoring", "name", impact.ArchetypeName)
			continue
		}
		w2.Write([]string{
			impact.ArchetypeName,
			fmt.Sprintf("%.1f", impact.Verbose.VCPU.Value),
			fmt.Sprintf("%.1f", impact.Verbose.Memory.Value),
			fmt.Sprintf("%.1f", impact.Impacts.GWP.Embodied.Max),
		})
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
	ArchetypeName string
	Impacts       struct {
		PE struct {
			Use struct {
				Max   float64 `json:"max"`
				Min   float64 `json:"min"`
				Value float64 `json:"value"`
			} `json:"use"`
		} `json:"pe"`
		GWP struct {
			Embodied struct {
				Max float64 `json:"max"`
			} `json:"embodied"`
		} `json:"gwp"`
	} `json:"impacts"`
	Verbose struct {
		VCPU struct {
			Value float64 `json:"value"`
		} `json:"vcpu"`
		AVGPower struct {
			Value JSONFloat `json:"value"`
		} `json:"avg_power"`
		Memory struct {
			Value float64 `json:"value"`
		} `json:"memory"`
		CPU_1 struct {
			CoreUnits struct {
				Value int `json:"value"`
			} `json:"core_units"`
			Family struct {
				Value string `json:"value"`
			} `json:"family"`
			Name struct {
				Value string `json:"value"`
			} `json:"name"`
			TDP struct {
				Value float64 `json:"value"`
			} `json:"tdp"`
			Threads struct {
				Value float64 `json:"value"`
			} `json:"threads"`
		} `json:"CPU-1"`
		SSD_1 struct {
			Capacity struct {
				Value float64 `json:"value"`
			} `json:"capacity"`
		} `json:"SSD-1"`
		HDD_1 struct {
			Capacity struct {
				Value float64 `json:"value"`
			} `json:"capacity"`
		} `json:"HDD-1"`
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
