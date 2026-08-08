package ffmpeg

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// libx264-only: handing -x264opts to libx265/libsvtav1 makes the encoder fail
// to open, killing the session rather than slowing it.
func TestSoftwareX264TuningArgs_OnlyLibx264(t *testing.T) {
	t.Parallel()
	for _, enc := range []string{"libx265", "libsvtav1", "h264_vaapi", "h264_nvenc", ""} {
		assert.Nil(t, softwareX264TuningArgs(enc, -1), "encoder %q must get no -x264opts", enc)
		assert.Nil(t, softwareX264TuningArgs(enc, 0), "encoder %q must get no -x264opts", enc)
	}
}

func TestSoftwareX264TuningArgs_SingleAndPerVariantForms(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{"-x264opts", x264SpeedTuning}, softwareX264TuningArgs("libx264", -1))
	// Each rung is addressed, or the tuning lands on one output only.
	assert.Equal(t, []string{"-x264opts:2", x264SpeedTuning}, softwareX264TuningArgs("libx264", 2))
}

// Pinned: the +24% came from this exact set, and dropping subme=0 or me=hex
// gives most of it back with nothing else in the suite noticing.
func TestX264SpeedTuning_KeepsTheMeasuredOptions(t *testing.T) {
	t.Parallel()
	for _, opt := range []string{"subme=0", "me=hex", "me_range=16", "rc_lookahead=10", "open_gop=0"} {
		assert.Contains(t, x264SpeedTuning, opt)
	}
}
