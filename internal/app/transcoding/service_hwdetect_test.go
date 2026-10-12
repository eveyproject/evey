package transcoding_test

import (
	"context"
	"testing"
	"time"

	apptranscode "github.com/eveyproject/evey/internal/app/transcoding"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	"github.com/eveyproject/evey/internal/domain/transcoding"
	tcfakes "github.com/eveyproject/evey/internal/domain/transcoding/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingDetector wraps a device list and counts how many times a full
// detection actually ran.
type countingDetector struct {
	devices []transcoding.HWDevice
	err     error
	calls   int
}

func (d *countingDetector) DetectDevices(_ context.Context) ([]transcoding.HWDevice, error) {
	d.calls++
	if d.err != nil {
		return nil, d.err
	}
	return d.devices, nil
}

func (d *countingDetector) SupportsCodec(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}

func newHWDetectService(hw transcoding.HWDetector, opts ...apptranscode.Option) *apptranscode.Service {
	return apptranscode.NewService(
		tcfakes.NewFakeTranscoder(), hw,
		tcfakes.NewFakeTranscodeJobRepository(), tcfakes.NewFakeTranscodeProfileRepository(),
		sharedfakes.NewFakeEventBus(), 2, opts...,
	)
}

// THE PROBE STORM (2026-07-15 evening). GetSystemInfo - polled by the admin
// panel - ran a full hardware detection on every call, and detection is not a
// lookup: it SPAWNS one probe ffmpeg per encoder to prove it really encodes.
// With the box already choking on a 4K software transcode, seven probe rounds
// failed "encoder present but real-encode probe failed" across the evening -
// each one noise in the logs AND stolen CPU at the worst possible moment.
// A GPU does not come and go: detection is cached and re-runs at most once
// per TTL.
func TestDetectHardware_CachedWithinTTL(t *testing.T) {
	hw := &countingDetector{devices: []transcoding.HWDevice{{Type: "vaapi", Name: "Intel GPU"}}}
	svc := newHWDetectService(hw, apptranscode.WithHWDetectTTL(80*time.Millisecond))

	d1, err := svc.DetectHardware(context.Background())
	require.NoError(t, err)
	d2, err := svc.DetectHardware(context.Background())
	require.NoError(t, err)

	assert.Equal(t, d1, d2)
	assert.Equal(t, 1, hw.calls, "a panel poll must serve the cached matrix, not spawn a fresh probe ffmpeg per encoder")

	time.Sleep(120 * time.Millisecond)
	_, err = svc.DetectHardware(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, hw.calls, "past the TTL the detection refreshes - hysteresis, not a permanent snapshot")
}

// A FAILED detection is never cached: a transient probe failure (the exact
// under-load case) must not stick a wrong matrix for a whole TTL.
func TestDetectHardware_ErrorsAreNotCached(t *testing.T) {
	hw := &countingDetector{err: assertErrHW("probe failed under load")}
	svc := newHWDetectService(hw, apptranscode.WithHWDetectTTL(time.Hour))

	_, err := svc.DetectHardware(context.Background())
	require.Error(t, err)

	hw.err = nil
	hw.devices = []transcoding.HWDevice{{Type: "vaapi", Name: "Intel GPU"}}
	devs, err := svc.DetectHardware(context.Background())
	require.NoError(t, err)
	assert.Len(t, devs, 1)
	assert.Equal(t, 2, hw.calls, "the failure was retried, not served from cache")
}

type assertErrHW string

func (e assertErrHW) Error() string { return string(e) }
