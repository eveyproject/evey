//go:build !linux

package loadtest

import (
	"context"
	"math/rand/v2"
)

// DeviceMapping says which block device a library location is read from. Only
// Linux can tell; elsewhere the mapping is empty and the report says so.
type DeviceMapping struct {
	Location string
	Device   string
	Resolved string
	Disks    []string
}

func libraryDevices(context.Context, []MediaCandidate, func(string) string, string, *rand.Rand) []DeviceMapping {
	return nil
}

func mappedDevices([]DeviceMapping) []string { return nil }

func allMapped([]DeviceMapping) bool { return false }
