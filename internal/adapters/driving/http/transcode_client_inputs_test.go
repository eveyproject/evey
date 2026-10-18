package httpapi

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
)

// The invariant behind every "master says X, segments carry Y" bug: the
// client profile a child URI re-derives must be the one the master was
// decided for. Bake the request's capabilities into a ManifestInput, emit the
// query every URI carries, parse it back as a request and compare what the
// decision would see on both sides. A new capability that reaches
// parseClientCapabilities without reaching bakeClientInputs fails here.
func TestChildURIsCarryTheClientProfile(t *testing.T) {
	master := httptest.NewRequest(http.MethodGet,
		"/api/v1/media/m1/hls/master.m3u8?videoCodecs=h264,vp9,av1&audioCodecs=aac,opus,ac3&containers=mp4,webm"+
			"&fmp4AudioPassthrough=mp3,opus&pixelFormats=yuv420p,hevc:yuv420p10le&hdr=1&maxAudioChannels=6&screenHeight=1080&maxHeight=1080&maxH264Level=41", nil)
	master.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:156.0) Gecko/20100101 Firefox/156.0")

	in := apptranscoding.ManifestInput{MediaID: "m1", StreamToken: "tok", AudioTrack: 0, SubtitleBurnInTrack: -1}
	bakeClientInputs(master, &in)
	suffix := apptranscoding.QuerySuffixFor(in)
	require.True(t, strings.HasPrefix(suffix, "?"), "suffix %q", suffix)

	child := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/audio_0_000.m4s"+suffix, nil)
	child.Header.Set("User-Agent", master.Header.Get("User-Agent"))

	want := parseClientCapabilities(master, hdrQuerySupported(master))
	got := parseClientCapabilities(child, hdrQuerySupported(child))
	assert.Equal(t, want, got, "the segment must decide for the same client as the master")
	assert.Equal(t, maxAudioChannelsQuery(master), maxAudioChannelsQuery(child))
	assert.Equal(t, hdrQuerySupported(master), hdrQuerySupported(child))

	// Every capability key the request carried is on the child URI.
	q, err := url.ParseQuery(strings.TrimPrefix(suffix, "?"))
	require.NoError(t, err)
	for _, key := range []string{"videoCodecs", "audioCodecs", "containers", "fmp4AudioPassthrough", "pixelFormats", "hdr", "maxAudioChannels", "screenHeight", "maxHeight", "maxH264Level"} {
		assert.NotEmptyf(t, q.Get(key), "child URI lost %q", key)
	}
}
