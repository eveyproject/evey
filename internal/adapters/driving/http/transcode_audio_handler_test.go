package httpapi

import (
	"context"
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
	calls []int
	err   error
}

func (f *fakeAudioEnsurer) EnsureAudioSegment(_ context.Context, _ apptranscoding.AudioFingerprint, segIdx int) error {
	f.calls = append(f.calls, segIdx)
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
