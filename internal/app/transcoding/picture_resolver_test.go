package transcoding

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// A host without a transcoder still draws the source corrections: an HDR film
// is folded to SDR, an anamorphic rip keeps its display aspect.
func TestPictureFor_SoftwareCorrections(t *testing.T) {
	sdr := PictureFor(sdrStreams("h264", "yuv420p", false))
	assert.Equal(t, transcoding.TopologySoftware, sdr.Topology)
	assert.False(t, sdr.ToneMap)
	assert.False(t, sdr.HW())
	assert.Equal(t, "h264", sdr.Codec)

	hdr := PictureFor(hdrStreams("hevc", "yuv420p10le", true))
	assert.Equal(t, transcoding.TopologySoftwareHDR, hdr.Topology)
	assert.True(t, hdr.ToneMap)
	assert.Equal(t, "pq", hdr.HDRTransfer)
	assert.False(t, hdr.HW())
}

// The picture takes the player's route: on a host that proves the GPU
// tone-map route, an HDR source lands on VAAPI/OpenCL exactly as a playback
// session does; an SDR source takes the cheaper QSV.
func TestResolvePicture_TakesThePlayersRoute(t *testing.T) {
	pr := twoBackendResolver()

	hdr := pr.ResolvePicture("m1", hdrStreams("hevc", "yuv420p10le", true), 480)
	assert.True(t, hdr.HW(), "the GPU draws the HDR picture")
	assert.Equal(t, transcoding.VAAPI, hdr.Accel)
	assert.Equal(t, "/dev/dri/renderD128", hdr.Device)
	assert.Equal(t, transcoding.TopologyGPUToneMap, hdr.Topology)
	assert.True(t, hdr.ToneMap)
	assert.Contains(t, hdr.Features, transcoding.FeatureTonemapOpenCL)

	sdr := pr.ResolvePicture("m2", sdrStreams("h264", "yuv420p", false), 480)
	assert.True(t, sdr.HW())
	assert.Equal(t, transcoding.QSV, sdr.Accel)
	assert.Equal(t, transcoding.TopologyZeroCopy, sdr.Topology)
	assert.False(t, sdr.ToneMap)
}

// The operator's toggle and the host's calibration gate the picture like a
// session: GPU off, or no capability, and the same source is drawn in software
// with its corrections intact.
func TestResolvePicture_SoftwareWhenTheGPUIsNotAllowed(t *testing.T) {
	off := twoBackendResolver()
	off.HWAccelEnabled = func() bool { return false }
	p := off.ResolvePicture("m1", hdrStreams("hevc", "yuv420p10le", true), 480)
	assert.False(t, p.HW())
	assert.Equal(t, transcoding.TopologySoftwareHDR, p.Topology)
	assert.True(t, p.ToneMap)

	bare := &PlanResolver{}
	q := bare.ResolvePicture("m1", hdrStreams("hevc", "yuv420p10le", true), 480)
	assert.False(t, q.HW())
	assert.True(t, q.ToneMap)
}

// A title the operator blocked off the GPU route stays off it for the picture too.
func TestResolvePicture_HonoursTheTitleBlock(t *testing.T) {
	pr := twoBackendResolver()
	pr.HWRouteBlocked = func(mediaID string) bool { return mediaID == "blocked" }
	assert.False(t, pr.ResolvePicture("blocked", sdrStreams("h264", "yuv420p", false), 480).HW())
	assert.True(t, pr.ResolvePicture("free", sdrStreams("h264", "yuv420p", false), 480).HW())
}

// Software strips the accelerator and nothing else.
func TestPicturePlan_Software(t *testing.T) {
	p := twoBackendResolver().ResolvePicture("m1", hdrStreams("hevc", "yuv420p10le", true), 480).Software()
	assert.False(t, p.HW())
	assert.Equal(t, transcoding.TopologySoftwareHDR, p.Topology)
	assert.True(t, p.ToneMap)
	assert.Empty(t, p.Accel)
	assert.Empty(t, p.Features)
}

// ApplyTo is the one projection onto the process options: what the builders
// read is what the resolver decided.
func TestPicturePlan_ApplyTo(t *testing.T) {
	p := twoBackendResolver().ResolvePicture("m1", hdrStreams("hevc", "yuv420p10le", true), 480)
	var o transcoding.HLSProcessOpts
	p.ApplyTo(&o)
	assert.Equal(t, transcoding.VAAPI, o.HWAccel)
	assert.Equal(t, "/dev/dri/renderD128", o.HWDevice)
	assert.True(t, o.ToneMap)
	assert.True(t, o.HWTonemap)
	assert.False(t, o.HWSoftwareDecode)
	assert.False(t, o.HDRSupported)
	assert.Equal(t, "h264", o.VideoCodec)

	var s transcoding.HLSProcessOpts
	p.Software().ApplyTo(&s)
	assert.Empty(t, s.HWAccel)
	assert.True(t, s.ToneMap)
	assert.False(t, s.HWTonemap)
}
