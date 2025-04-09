package gcp

type Zone struct {
	Name   string
	Region string
}

type Zones []Zone

func (zs Zones) GetRegion(location string) string {
	if location == "global" {
		return "global"
	}

	for _, zone := range zs {
		if zone.Region == location {
			return zone.Region
		}

		if zone.Name == location {
			return zone.Region
		}
	}

	return "global"
}

func (zs Zones) IsValidZone(location string) bool {
	for _, zone := range zs {
		if zone.Name == location {
			return true
		}
	}

	return false
}
