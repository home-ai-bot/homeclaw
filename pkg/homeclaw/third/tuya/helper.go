package tuya

import (
	tuya "github.com/AlexxIT/go2rtc/pkg/tuya"
)

// GetRegionByHost finds a region by its host
func GetRegionByHost(host string) *tuya.Region {
	for _, r := range tuya.AvailableRegions {
		if r.Host == host {
			return &r
		}
	}
	return nil
}

// GetRegionByName finds a region by its name
func GetRegionByName(name string) *tuya.Region {
	for _, r := range tuya.AvailableRegions {
		if r.Name == name {
			return &r
		}
	}
	return nil
}
