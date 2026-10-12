package mkvgo

import (
	"context"
	"testing"

	extmkv "github.com/gravity-zero/mkvgo/mkv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
)

// keepTracksFor always keeps the (first) video track and every subtitle
// track - only the audio set narrows to the requested ordinal. This mirrors
// resolveAudioTrack's 0-based-among-audio-streams numbering exactly, so a
// trimmed plan agrees with whatever ?audioTrack=/profile default the ffmpeg
// pipeline would have picked for the same request.
func TestKeepTracksFor(t *testing.T) {
	tracks := []extmkv.Track{
		{ID: 1, Type: extmkv.VideoTrack},
		{ID: 2, Type: extmkv.AudioTrack}, // audio ordinal 0
		{ID: 3, Type: extmkv.AudioTrack}, // audio ordinal 1
		{ID: 4, Type: extmkv.SubtitleTrack},
		{ID: 5, Type: extmkv.SubtitleTrack},
	}

	got := keepTracksFor(tracks, 1)
	assert.ElementsMatch(t, []uint64{1, 3, 4, 5}, got, "video + audio ordinal 1 (track 3) + every subtitle")

	got0 := keepTracksFor(tracks, 0)
	assert.ElementsMatch(t, []uint64{1, 2, 4, 5}, got0, "audio ordinal 0 (track 2)")
}

func TestKeepTracksFor_OutOfRangeOrdinalKeepsNothingAudio(t *testing.T) {
	tracks := []extmkv.Track{
		{ID: 1, Type: extmkv.VideoTrack},
		{ID: 2, Type: extmkv.AudioTrack},
	}
	// An out-of-range ordinal (stale request against a re-scanned file with
	// fewer tracks now) must not silently keep every audio track - that would
	// defeat the trim. It degrades to video + subs only, same as "no audio".
	got := keepTracksFor(tracks, 5)
	assert.ElementsMatch(t, []uint64{1}, got)
}

func TestKeepTracksFor_NoVideoTrack(t *testing.T) {
	tracks := []extmkv.Track{{ID: 1, Type: extmkv.AudioTrack}}
	got := keepTracksFor(tracks, 0)
	assert.ElementsMatch(t, []uint64{1}, got, "audio-only source: no video track to keep, audio still selected")
}

// Plan wires the trim in, best-effort: a track-listing failure must not
// refuse the plan (same fail-soft contract as SynthesizeIndex/AudioPresentationShift).
func TestOnDemandPackager_Plan_WiresKeepAudioTrack(t *testing.T) {
	p := NewOnDemandPackager()
	defer p.Close()
	p.audioDelays = func(context.Context, string) (map[uint64]int64, error) { return nil, nil }
	var gotPath string
	p.trackList = func(_ context.Context, path string) ([]extmkv.Track, error) {
		gotPath = path
		return []extmkv.Track{{ID: 1, Type: extmkv.VideoTrack}, {ID: 2, Type: extmkv.AudioTrack}}, nil
	}

	idx := 0
	_, err := p.Plan(context.Background(), "does-not-exist.mkv", apptranscoding.OnDemandOptions{KeepAudioTrack: &idx})
	require.Error(t, err, "PlanHLS still fails fast on an unreadable source")
	assert.Equal(t, "does-not-exist.mkv", gotPath, "the track list runs before PlanHLS builds the plan")
}

func TestOnDemandPackager_Plan_NilKeepAudioTrackSkipsTrackList(t *testing.T) {
	p := NewOnDemandPackager()
	defer p.Close()
	called := false
	p.trackList = func(context.Context, string) ([]extmkv.Track, error) {
		called = true
		return nil, nil
	}
	_, _ = p.Plan(context.Background(), "movie.mkv", apptranscoding.OnDemandOptions{})
	assert.False(t, called, "no restriction requested: today's untrimmed behaviour, zero extra read")
}

func TestOnDemandCacheKey_KeepAudioTrackDistinguishesPlans(t *testing.T) {
	idx0, idx1 := 0, 1
	kFull := onDemandCacheKey("movie.mkv", apptranscoding.OnDemandOptions{})
	k0 := onDemandCacheKey("movie.mkv", apptranscoding.OnDemandOptions{KeepAudioTrack: &idx0})
	k1 := onDemandCacheKey("movie.mkv", apptranscoding.OnDemandOptions{KeepAudioTrack: &idx1})
	assert.NotEqual(t, kFull, k0, "a trimmed plan must not alias the untrimmed one")
	assert.NotEqual(t, k0, k1, "two different trims must not alias each other")
}
