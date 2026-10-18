package transcoding_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"

	apptranscode "github.com/eveyproject/evey/internal/app/transcoding"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/transcoding"
)

func pixStreams(codec string, depth int, pixFmt string) []*library.MediaStream {
	v := &library.MediaStream{StreamType: "video", Codec: codec, Width: new(1920), Height: new(1080), IsDefault: true}
	if depth > 0 {
		v.BitDepth = new(depth)
	}
	if pixFmt != "" {
		v.PixelFormat = new(pixFmt)
	}
	return []*library.MediaStream{v, {StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true}}
}

func pixClient(formats ...string) transcoding.ClientProfile {
	c := transcoding.DefaultWebProfile
	c.VideoCodecs = []string{"h264", "hevc", "vp9", "av1"}
	c.Containers = []string{"mp4", "mkv"}
	c.PixelFormats = transcoding.NormalizePixelFormats(formats)
	return c
}

func pixFormatRefused(t *testing.T, streams []*library.MediaStream, client transcoding.ClientProfile) bool {
	t.Helper()
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", client, 0)
	return slices.Contains(d.TranscodeReasons, "pixel_format_unsupported")
}

// A client that declares its pixel formats is judged on them, not on what
// browsers decode. A TV that takes 4:2:2 10-bit HEVC and 10-bit H.264 keeps the
// copy the browser rule refused; a device without a 10-bit decoder loses the
// copy the browser rule granted. The declaration may name one codec only: a
// Main10 HEVC decoder says nothing about the H.264 one.
func TestPixelFormat_ClientDeclarationDecides(t *testing.T) {
	tv := pixClient("yuv420p", "yuv420p10", "yuv422p", "yuv422p10")
	assert.False(t, pixFormatRefused(t, pixStreams("hevc", 10, "yuv422p10le"), tv), "4:2:2 10-bit HEVC on a device that decodes it")
	assert.False(t, pixFormatRefused(t, pixStreams("h264", 10, "yuv420p10le"), tv), "Hi10P on a device that decodes it")
	assert.True(t, pixFormatRefused(t, pixStreams("hevc", 12, "yuv420p12le"), tv), "12-bit is not declared")
	assert.True(t, pixFormatRefused(t, pixStreams("hevc", 8, "yuv444p"), tv), "4:4:4 is not declared")

	eightBit := pixClient("yuv420p")
	assert.True(t, pixFormatRefused(t, pixStreams("hevc", 10, "yuv420p10le"), eightBit), "10-bit HEVC on an 8-bit-only decoder")
	assert.True(t, pixFormatRefused(t, pixStreams("hevc", 10, ""), eightBit), "depth alone names 10-bit 4:2:0")
	assert.False(t, pixFormatRefused(t, pixStreams("h264", 8, "yuvj420p"), eightBit), "full range is the same decoder")
	assert.False(t, pixFormatRefused(t, pixStreams("h264", 0, ""), eightBit), "an unprobed source is trusted, as without a declaration")

	hevcOnly := pixClient("yuv420p", "hevc:yuv420p10", "h265:p010le")
	assert.Equal(t, []string{"yuv420p", "hevc:yuv420p10"}, hevcOnly.PixelFormats, "aliases fold, duplicates drop")
	assert.False(t, pixFormatRefused(t, pixStreams("hevc", 10, "p010le"), hevcOnly))
	assert.True(t, pixFormatRefused(t, pixStreams("h264", 10, "yuv420p10le"), hevcOnly), "Main10 HEVC says nothing of H.264")
}

// Without a declaration the built-in rule is untouched, case for case: every
// client shipped today sends none, and must see the same decisions.
func TestPixelFormat_NoDeclarationKeepsTheBrowserRule(t *testing.T) {
	none := pixClient()
	assert.Nil(t, none.PixelFormats)
	assert.Nil(t, transcoding.NormalizePixelFormats([]string{"", " ", "hevc:"}), "nothing usable is no declaration")
	for _, tc := range []struct {
		codec   string
		depth   int
		pixFmt  string
		refused bool
	}{
		{"h264", 8, "yuv420p", false},
		{"h264", 10, "yuv420p10le", true},
		{"h264", 10, "", true},
		{"h264", 8, "yuv422p", true},
		{"hevc", 10, "yuv420p10le", false},
		{"hevc", 10, "p010le", false},
		{"hevc", 12, "yuv420p12le", true},
		{"hevc", 10, "yuv422p10le", true},
		{"vp9", 10, "", false},
		{"av1", 0, "", false},
	} {
		assert.Equalf(t, tc.refused, pixFormatRefused(t, pixStreams(tc.codec, tc.depth, tc.pixFmt), none), "%s %d %q", tc.codec, tc.depth, tc.pixFmt)
	}
}
