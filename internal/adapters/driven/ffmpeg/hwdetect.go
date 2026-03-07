package ffmpeg

import (
	"context"
	"errors"

	"github.com/gravity-zero/evey/internal/domain/transcoding"
)

type HWDetector struct{}

func NewHWDetector() *HWDetector {
	return &HWDetector{}
}

func (d *HWDetector) DetectDevices(_ context.Context) ([]transcoding.HWDevice, error) {
	// TODO: check /dev/dri/ for VAAPI, run nvidia-smi for NVENC, check for QSV
	return nil, errors.New("TODO: implement hardware detection")
}

func (d *HWDetector) SupportsCodec(_ context.Context, _, _ string) (bool, error) {
	// TODO: query ffmpeg -encoders for codec support on device
	return false, errors.New("TODO: implement codec support check")
}
