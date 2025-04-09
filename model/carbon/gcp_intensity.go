package carbon

import (
	"embed"
	"encoding/csv"
	"strconv"

	"github.com/superdango/cloud-carbon-exporter/internal/must"
)

//go:embed data/gcp_region_carbon_info_2023.csv
var carboninfo embed.FS

// NewGCPCarbonIntensityMap loads and parse official carbon data provided by GCP
// https://github.com/GoogleCloudPlatform/region-carbon-info
// Should be updated each year.
func NewGCPCarbonIntensityMap() IntensityMap {
	f, err := carboninfo.Open("data/gcp_region_carbon_info_2023.csv")
	must.NoError(err)

	intensityData := csv.NewReader(f)
	locations, err := intensityData.ReadAll()
	must.NoError(err)

	intensity := make(IntensityMap)

	for line, location := range locations {
		// skip csv header
		if line == 0 {
			continue
		}
		must.Assert(len(location) == 4, "csv line must be 4 fields length")
		region := location[0]
		co2eqbykwh := strToFloat64(location[3])

		intensity[region] = co2eqbykwh
	}

	intensity["emea"] = intensity.Average("eu", "me", "af")
	intensity["apac"] = intensity.Average("as", "au")
	intensity["amer"] = intensity.Average("no", "so", "us")

	//https://cloud.google.com/storage/docs/locations#predefined
	intensity["asia1"] = intensity.Average("asia-east1", "asia-southeast1")
	intensity["eur4"] = intensity.Average("europe-north1", "europe-west4")
	intensity["eur5"] = intensity.Average("europe-west1", "europe-west2")
	intensity["eur7"] = intensity.Average("europe-west2", "europe-west3")
	intensity["eur8"] = intensity.Average("europe-west3", "europe-west6")
	intensity["nam4"] = intensity.Average("us-central1", "us-east1")

	//https://cloud.google.com/storage/docs/locations#location-mr
	intensity["asia"] = intensity.Average("as")
	intensity["eu"] = intensity.Average("eu")
	intensity["us"] = intensity.Average("us")

	intensity["global"] = intensity.Average("emea", "apac", "amer")

	return intensity
}

func strToFloat64(s string) float64 {
	f, err := strconv.ParseFloat(s, 64)
	must.NoError(err)
	return f
}
