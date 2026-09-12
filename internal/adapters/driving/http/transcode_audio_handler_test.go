package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	"github.com/evey-project/evey/internal/config"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

// ----------------------------------------------------------------
// Test-local fakes for audio handler tests
// ----------------------------------------------------------------

// fakeAudioEnsurer implements AudioSegmentEnsurer for tests.
// It never actually spawns ffmpeg - it just records calls and
// optionally returns a preset error.
type fakeAudioEnsurer struct {
	calls     []int // segment indices requested
	initCalls int
	err       error
}

func (f *fakeAudioEnsurer) EnsureAudioSegment(_ context.Context, _ apptranscoding.AudioFingerprint, segIdx int) error {
	f.calls = append(f.calls, segIdx)
	return f.err
}

func (f *fakeAudioEnsurer) EnsureAudioInit(_ context.Context, _ apptranscoding.AudioFingerprint) error {
	f.initCalls++
	return f.err
}

// ----------------------------------------------------------------
// Construction helpers
// ----------------------------------------------------------------

// testMediaItemWithAudioStreams builds a 60 s media item with two audio tracks.
func testMediaItemWithAudioStreams() (*domainlibrary.MediaItem, []*domainlibrary.MediaStream) {
	dur, _ := shared.NewDurationMs(60_000)
	item := &domainlibrary.MediaItem{
		ID:        "m1",
		LibraryID: "lib1",
		File: &domainlibrary.MediaFile{
			Path:     "/test.mkv",
			Duration: dur,
		},
	}
	streams := []*domainlibrary.MediaStream{
		{StreamType: "video", Codec: "h264"},
		{StreamType: "audio", Codec: "aac", Language: "en"},
		{StreamType: "audio", Codec: "ac3", Language: "fr"},
	}
	return item, streams
}

// buildAudioServe returns an audioServe with the feature flag enabled by default.
func buildAudioServe(
	t *testing.T,
	flagOn bool,
	libSvc MediaStreamsGetter,
	libAccess LibraryAccessChecker,
	parental ParentalChecker,
	audioOrch AudioSegmentEnsurer,
) *audioServe {
	t.Helper()
	cacheDir := t.TempDir()
	cache, err := apptranscoding.NewSegmentCache(cacheDir + "/audio")
	require.NoError(t, err)

	cfg := &config.Config{}
	cfg.Transcoding.AudioRenditionEnabled = flagOn

	return &audioServe{
		cfg:                  cfg,
		libSvc:               libSvc,
		libraryAccess:        libAccess,
		parental:             parental,
		manifestGen:          apptranscoding.NewManifestGenerator(),
		cache:                cache,
		audioOrch:            audioOrch,
		audioNormalizeGetter: nil,
		segmentWaitTimeout:   50 * time.Millisecond,
	}
}

// ----------------------------------------------------------------
// Tests
// ----------------------------------------------------------------
//
// Coverage for the auth/parental/flag-off/orchestrator-error paths through
// servePlaylistFile / serveInitFile / serveSegmentFile lives in
// transcode_audio_dispatch_test.go, exercised via hlsVODServe.dispatchAudio*
// (the actual production entry point).

// buildAudioRenditions correctly extracts audio tracks only.
func TestBuildAudioRenditions_FiltersAudioOnly(t *testing.T) {
	streams := []*domainlibrary.MediaStream{
		{StreamType: "video", Codec: "h264"},
		{StreamType: "audio", Codec: "aac", Language: "en", Title: "English"},
		{StreamType: "subtitle"},
		{StreamType: "audio", Codec: "ac3", Language: "fr"},
	}
	renditions := buildAudioRenditions(streams)

	require.Len(t, renditions, 2)
	assert.Equal(t, 0, renditions[0].SourceTrackIdx)
	assert.Equal(t, "English", renditions[0].Name)
	assert.Equal(t, "en", renditions[0].LanguageCode)
	assert.Equal(t, 1, renditions[1].SourceTrackIdx)
	assert.Equal(t, "fr", renditions[1].LanguageCode)
}

// The hybrid mkvgo rung (video copied by the packager + audio re-encoded here)
// must carry the SOURCE's channel layout: MaxAudioChannels 0 makes ffmpeg emit
// `-ac 2`, silently downmixing a 5.1 AC-3 track to stereo. The muxed ffmpeg path
// it replaces preserves 5.1, so the fingerprint has to derive the channel count
// from the requested track instead of hardcoding the stereo default.
func TestBuildAudioFingerprint_DerivesChannelsFromSourceTrack(t *testing.T) {
	six := 6
	two := 2
	item, _ := testMediaItemWithAudioStreams()
	streams := []*domainlibrary.MediaStream{
		{StreamType: "video", Codec: "h264"},
		{StreamType: "audio", Codec: "aac", Language: "en", Channels: &two}, // audio idx 0
		{StreamType: "audio", Codec: "ac3", Language: "fr", Channels: &six}, // audio idx 1 (5.1)
	}
	h := buildAudioServe(t, true, &testFakeStreamsGetter{item: item, streams: streams}, nil, nil, nil)
	r := httptest.NewRequest(http.MethodGet, "/hls/audio_1.m3u8", nil)

	fp := h.buildAudioFingerprint(r, "m1", item, 1, 0, "") // the 5.1 French track
	if fp.MaxAudioChannels != 6 {
		t.Errorf("MaxAudioChannels = %d, want 6 (source 5.1 must not be downmixed)", fp.MaxAudioChannels)
	}

	fpStereo := h.buildAudioFingerprint(r, "m1", item, 0, 0, "")
	if fpStereo.MaxAudioChannels != 2 {
		t.Errorf("MaxAudioChannels = %d, want 2 for the stereo track", fpStereo.MaxAudioChannels)
	}
}

// The declared media duration rides the fingerprint (transport only, never
// hashed) so the orchestrator can BOUND the silence generator it spawns when
// the audio track ends before that duration. Without it the orchestrator has
// no bound and must refuse the tail instead of synthesizing it.
func TestBuildAudioFingerprint_CarriesSourceDurationForSilenceBound(t *testing.T) {
	item, streams := testMediaItemWithAudioStreams() // 60 s declared duration
	h := buildAudioServe(t, true, &testFakeStreamsGetter{item: item, streams: streams}, nil, nil, nil)
	r := httptest.NewRequest(http.MethodGet, "/hls/audio_0.m3u8", nil)

	fp := h.buildAudioFingerprint(r, "m1", item, 0, 0, "")
	assert.InDelta(t, 60.0, fp.SourceDurationSec, 0.001,
		"the declared duration must reach the orchestrator to bound a silence tail")
}

// The rendition must not emit more channels than a browser can decode. Deriving
// the count from the SOURCE alone (my fix for the stereo downmix) went too far the
// other way: a Dolby Atmos 7.1 track yielded `-ac 8`, the SourceBuffer refused the
// stream, and the title never started at all. 5.1 (6ch) is the real ceiling.
func TestBuildAudioFingerprint_CapsChannelsAtSurround(t *testing.T) {
	eight := 8 // 7.1 Atmos source
	six := 6
	item, _ := testMediaItemWithAudioStreams()
	streams := []*domainlibrary.MediaStream{
		{StreamType: "video", Codec: "h264"},
		{StreamType: "audio", Codec: "truehd", Channels: &eight}, // audio idx 0: 7.1
		{StreamType: "audio", Codec: "ac3", Channels: &six},      // audio idx 1: 5.1
	}
	h := buildAudioServe(t, true, &testFakeStreamsGetter{item: item, streams: streams}, nil, nil, nil)
	r := httptest.NewRequest(http.MethodGet, "/hls/audio_0.m3u8", nil)

	fp := h.buildAudioFingerprint(r, "m1", item, 0, 0, "")
	if fp.MaxAudioChannels > 6 {
		t.Errorf("MaxAudioChannels = %d for a 7.1 source: no browser decodes AAC 7.1 - "+
			"the stream is refused and playback never starts", fp.MaxAudioChannels)
	}

	// 5.1 IS decodable: it must survive untouched (the bug this replaced).
	if got := h.buildAudioFingerprint(r, "m1", item, 1, 0, "").MaxAudioChannels; got != 6 {
		t.Errorf("MaxAudioChannels = %d for a 5.1 source, want 6 (must not be downmixed)", got)
	}
}

// A client that declares a channel CEILING must get it. Deriving the rendition
// from the source alone ignored ?maxAudioChannels entirely, which is what made
// the cast receiver's stereo fallback a placebo: after an error it renegotiated
// the master down to 2, the rendition kept serving the source's 5.1, and the
// rescue reload failed exactly like the load it was rescuing (2026-08-08,
// Chromecast HD - three casts, not one second of playback).
func TestBuildAudioFingerprint_ClientCeilingCapsTheSourceLayout(t *testing.T) {
	six := 6
	item, _ := testMediaItemWithAudioStreams()
	streams := []*domainlibrary.MediaStream{
		{StreamType: "video", Codec: "h264"},
		{StreamType: "audio", Codec: "ac3", Language: "fr", Channels: &six}, // 5.1 source
	}
	h := buildAudioServe(t, true, &testFakeStreamsGetter{item: item, streams: streams}, nil, nil, nil)

	stereo := httptest.NewRequest(http.MethodGet, "/hls/audio_0.m3u8?maxAudioChannels=2", nil)
	if got := h.buildAudioFingerprint(stereo, "m1", item, 0, 0, "").MaxAudioChannels; got != 2 {
		t.Errorf("MaxAudioChannels = %d for a client asking for stereo, want 2: "+
			"a stereo-only sink cannot decode the 5.1 this serves instead", got)
	}

	// No ceiling declared (every web player): the source layout, untouched.
	none := httptest.NewRequest(http.MethodGet, "/hls/audio_0.m3u8", nil)
	if got := h.buildAudioFingerprint(none, "m1", item, 0, 0, "").MaxAudioChannels; got != 6 {
		t.Errorf("MaxAudioChannels = %d without a declared ceiling, want 6 (source layout)", got)
	}

	// A ceiling ABOVE the source never invents channels.
	high := httptest.NewRequest(http.MethodGet, "/hls/audio_0.m3u8?maxAudioChannels=8", nil)
	if got := h.buildAudioFingerprint(high, "m1", item, 0, 0, "").MaxAudioChannels; got != 6 {
		t.Errorf("MaxAudioChannels = %d for a ceiling above the source, want 6", got)
	}
}
