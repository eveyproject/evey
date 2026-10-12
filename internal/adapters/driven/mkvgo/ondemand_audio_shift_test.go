package mkvgo

import (
	"context"
	"testing"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// audioPresentationShift feeds mp4.Options.AudioPresentationShift straight
// from AudioStartDelays: only tracks past the defect floor are shifted
// (codec priming stays untouched), and the sign is NOT negated - unlike the
// persistent retimer, AudioPresentationShift takes AudioStartDelays' value
// directly (positive = presented earlier).
func TestAudioPresentationShift_FloorsAlignedTracks(t *testing.T) {
	got := audioPresentationShift(map[uint64]int64{2: 900_000_000, 3: 20_000_000, 4: 0})
	require.Len(t, got, 1)
	assert.Equal(t, int64(900_000_000), got[2], "shift takes AudioStartDelays' sign directly, not negated")
}

func TestAudioPresentationShift_AllAlignedReturnsNil(t *testing.T) {
	assert.Nil(t, audioPresentationShift(map[uint64]int64{2: 10_000_000}))
	assert.Nil(t, audioPresentationShift(nil))
}

// Plan wires the per-track shift into the packaged presentation for a
// Matroska source, best-effort: a probe failure must not fail the plan (the
// packager never blocks playback on a diagnostic step), it just serves
// without the resync - the pre-existing desynced-copy behaviour.
func TestOnDemandPackager_Plan_WiresAudioShift(t *testing.T) {
	p := NewOnDemandPackager()
	defer p.Close()
	var gotPath string
	p.audioDelays = func(_ context.Context, path string) (map[uint64]int64, error) {
		gotPath = path
		return map[uint64]int64{2: 500_000_000}, nil
	}

	_, err := p.Plan(context.Background(), "does-not-exist.mkv", apptranscoding.OnDemandOptions{})
	require.Error(t, err, "PlanHLS still fails fast on an unreadable source")
	assert.Equal(t, "does-not-exist.mkv", gotPath, "the delay probe runs before PlanHLS builds the plan")
}

func TestOnDemandPackager_Plan_SkipsAudioDelayProbeForMP4(t *testing.T) {
	p := NewOnDemandPackager()
	defer p.Close()
	called := false
	p.audioDelays = func(_ context.Context, _ string) (map[uint64]int64, error) {
		called = true
		return nil, nil
	}

	_, _ = p.Plan(context.Background(), "movie.mp4", apptranscoding.OnDemandOptions{})
	assert.False(t, called, "AudioStartDelays is Matroska-only, must not run against an MP4 source")
}
