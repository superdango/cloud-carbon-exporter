package carbon

func NewScalewayCloudCarbonFootprintIntensityMap() IntensityMap {
	scwIntensityMap := IntensityMap{
		"nl-ams": 236.0,
		"pl-waw": 311.0,
		"fr-par": 51.1,
	}

	scwIntensityMap["global"] = scwIntensityMap.Average()
	scwIntensityMap["emea"] = scwIntensityMap.Average("fr", "pl", "nl")

	return scwIntensityMap
}
