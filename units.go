package cloudcarbonexporter

type Energy float64

// CO2eq in gram/second
type CO2eq float64

// KgCO2eq_day converts CO2eq in kgCO2eq/day
func (co2 CO2eq) KgCO2eq_day() float64 {
	return float64(co2) * 60 * 60 * 24 / 1000
}
