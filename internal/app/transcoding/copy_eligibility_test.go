package transcoding

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCopyRungEligible(t *testing.T) {
	l40 := 40
	assert.True(t, CopyRungEligible("h264", &l40, false), "H.264 8-bit with a known level is copyable")
	assert.True(t, CopyRungEligible("H264", &l40, false), "codec match is case-insensitive")

	// The guard: no level ⟹ no copy (can't advertise an accurate avc1.PPCCLL).
	assert.False(t, CopyRungEligible("h264", nil, false), "unknown level must gate copy off")
	// Scope guards.
	assert.False(t, CopyRungEligible("hevc", &l40, false), "non-H.264 is out of copy scope")
	assert.False(t, CopyRungEligible("h264", &l40, true), "10-bit is out of copy scope")
}
