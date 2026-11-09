package httpapi

import (
	"net/http"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
)

// A browser playing an AC-3 track through the wasm decoder gets a video-only
// master whose variant URIs carry wasmAudio=1. Those URIs must not also carry
// an ad=transcode pin: the pin forces a re-encode on the variant request, which
// then hashes another fingerprint than the one the master sealed, and every
// variant answers 410 - the player reloads the master and loops for ever.
func TestHLSVOD_WasmAudioMasterVariantsServe(t *testing.T) {
	ch := func(n int) *int { return &n }
	vw, vh, lvl := 1920, 1080, 150
	streams := []*domainlibrary.MediaStream{
		{StreamType: domainlibrary.StreamTypeVideo, Codec: "hevc", Profile: "Main 10", Width: &vw, Height: &vh, Level: &lvl, Keyframes: []int64{0, 6000, 12000}},
		{StreamType: domainlibrary.StreamTypeAudio, Codec: "eac3", Channels: ch(6), IsDefault: true},
	}
	h, _ := buildVODHandler(t,
		&testFakeStreamsGetter{item: testMediaItemWith1Minute(), streams: streams},
		allowAll(),
		testFakeParentalChecker{},
	)
	h.onDemand = &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}

	const q = "wasmAudio=1&audioTrack=0&videoCodecs=h264,vp9,av1&audioCodecs=aac,mp3,opus,flac&containers=mp4,webm&maxAudioChannels=2"
	w := doVODRequestQ(h, "m1", "master.m3u8", q)
	require.Equal(t, http.StatusOK, w.Code)
	master := w.Body.String()
	require.Contains(t, master, "#EVEY-AUDIO-URI", "test rig: the request must take the wasm-audio master:\n%s", master)

	m := regexp.MustCompile(`(?m)^stream_0\.m3u8\?(\S+)$`).FindStringSubmatch(master)
	require.NotNil(t, m, "the wasm master keeps the ffmpeg video ladder here:\n%s", master)
	assert.Contains(t, m[1], "wasmAudio=1")
	assert.NotContains(t, m[1], "ad=transcode", "a wasm playback carries no audio re-encode pin")

	w = doVODRequestQ(h, "m1", "stream_0.m3u8", m[1])
	assert.Equal(t, http.StatusOK, w.Code, "the variant the wasm master names must serve, not 410: %s", w.Body.String())

	// The segments carry the same query and go through the same seal check.
	for _, file := range []string{"stream_0_init.mp4", "stream_0_000.m4s"} {
		w = doVODRequestQ(h, "m1", file, m[1])
		assert.NotEqual(t, http.StatusGone, w.Code, "%s must not be refused as a stale rendition", file)
	}
}
