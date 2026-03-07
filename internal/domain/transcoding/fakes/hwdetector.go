package fakes

import (
	"context"

	"github.com/gravity-zero/evey/internal/domain/transcoding"
)

type FakeHWDetector struct {
	Devices []transcoding.HWDevice
	Err     error
}

func NewFakeHWDetector() *FakeHWDetector {
	return &FakeHWDetector{}
}

func (d *FakeHWDetector) DetectDevices(_ context.Context) ([]transcoding.HWDevice, error) {
	if d.Err != nil {
		return nil, d.Err
	}
	return d.Devices, nil
}

func (d *FakeHWDetector) SupportsCodec(_ context.Context, deviceType, codec string) (bool, error) {
	for _, dev := range d.Devices {
		if dev.Type == deviceType {
			for _, c := range dev.SupportedCodecs {
				if c == codec {
					return true, nil
				}
			}
		}
	}
	return false, nil
}
