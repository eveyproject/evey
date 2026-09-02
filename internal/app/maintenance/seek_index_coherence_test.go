package maintenance_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appmaintenance "github.com/evey-project/evey/internal/app/maintenance"
	"github.com/evey-project/evey/internal/domain/mkv"
)

// fakeCoherenceProber answers a canned (lag, confidence) per path and counts
// the calls, so a test can prove WHICH files were compared - the whole point
// of the trigger being tested here.
type fakeCoherenceProber struct {
	mu     sync.Mutex
	byPath map[string][2]float64
	calls  map[string]int
}

func (f *fakeCoherenceProber) ProbeAudioTrackLagSec(_ context.Context, p string) (float64, float64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[p]++
	v := f.byPath[p]
	return v[0], v[1], nil
}

func (f *fakeCoherenceProber) callsFor(p string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[p]
}

// The comparison must run on every file with two or more audio tracks, NOT
// only on those declaring an offset. This is the 2026-07-13 shape frozen: the
// repair pulled the dub back onto the picture, so the file declares NOTHING
// any more (track 2 at 0.000 s, track 3 keeping its 0.020 s priming) - and
// the old trigger, keyed on a declared offset, would have classed all 98
// rewritten files as healthy forever.
func TestSeekIndexService_CoherenceRunsOnEveryMultiTrackFile(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "retimed_dub.mkv")
	single := filepath.Join(dir, "one_track.mkv")
	aligned := filepath.Join(dir, "two_tracks_fine.mkv")
	for _, p := range []string{victim, single, aligned} {
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	}
	reidx := &fakeReindexer{hasIndex: map[string]bool{victim: true, single: true, aligned: true}}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{
		{MediaID: "m-victim", Path: victim},
		{MediaID: "m-single", Path: single},
		{MediaID: "m-aligned", Path: aligned},
	}}
	coh := &fakeCoherenceProber{byPath: map[string][2]float64{
		victim:  {11.94, 0.79}, // the dub is 12 s early: its tracks disagree
		aligned: {0.02, 0.9},   // two tracks of one film line up
	}}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil).
		WithDiagnoser(fakeDiagnoser{byPath: map[string]*mkv.Diagnosis{
			victim:  {Healthy: true, AudioDelaysMs: map[uint64]int64{2: 0, 3: 20}},
			single:  {Healthy: true, AudioDelaysMs: map[uint64]int64{2: 0}},
			aligned: {Healthy: true, AudioDelaysMs: map[uint64]int64{2: 0, 3: 0}},
		}}).
		WithAudioCoherenceProber(coh)

	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitIdle(t, svc)

	res, _, ok := svc.CachedScan("")
	require.True(t, ok)
	byPath := map[string]appmaintenance.SeekStatus{}
	for _, it := range res.Items {
		byPath[it.Path] = it
	}

	assert.Equal(t, 1, coh.callsFor(victim), "a two-track file declaring NO offset must still be compared")
	assert.Equal(t, 1, coh.callsFor(aligned), "and so must every other multi-track file")
	assert.Zero(t, coh.callsFor(single), "a single-track file has nothing to compare - no subprocess for it")

	v := byPath[victim]
	assert.True(t, v.AudioTracksChecked)
	assert.InDelta(t, 11.94, v.AudioTracksLagSec, 1e-9)
	assert.Equal(t, 2, v.AudioTracks)
	assert.Equal(t, appmaintenance.VerdictInformational, v.Verdict(), "reported, never repaired")

	a := byPath[aligned]
	assert.True(t, a.AudioTracksChecked, "a coherent file is PROVEN coherent, not merely unexamined")
	assert.Equal(t, appmaintenance.VerdictHealthy, a.Verdict())
	assert.False(t, byPath[single].AudioTracksChecked)
}

// A track at exactly 0 next to a sibling keeping its priming offset is the
// mark a retime leaves. It names the touched files for the cost of a header
// read, which is what the 2026-07-13 forensics could only do by mtime.
func TestSeekIndexService_RetimedTrackIsRecognisedByItsSignature(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "angel.mkv")
	require.NoError(t, os.WriteFile(victim, []byte("x"), 0o644))
	svc := appmaintenance.NewSeekIndexService(
		&fakeReindexer{hasIndex: map[string]bool{victim: true}},
		fakeCatalog{files: []appmaintenance.MediaRef{{MediaID: "m1", Path: victim}}}, nil).
		WithDiagnoser(fakeDiagnoser{byPath: map[string]*mkv.Diagnosis{
			// Angel has fallen, as found: "track 2 fre delay=0.000s / track 3 eng delay=0.020s".
			victim: {Healthy: true, AudioDelaysMs: map[uint64]int64{2: 0, 3: 20}},
		}})
	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitIdle(t, svc)
	res, _, _ := svc.CachedScan("")
	require.Len(t, res.Items, 1)
	assert.True(t, res.Items[0].AudioTrackZeroed)
	assert.Equal(t, appmaintenance.VerdictInformational, res.Items[0].Verdict(),
		"a remark for the operator: the value removed cannot be recovered here")
	assert.True(t, res.Items[0].Ackable())
}

func TestRetimeSignature(t *testing.T) {
	cases := map[string]struct {
		delays map[uint64]int64
		want   bool
	}{
		"one zeroed, sibling keeps priming":  {map[uint64]int64{2: 0, 3: 20}, true},
		"one zeroed, sibling slightly early": {map[uint64]int64{2: 0, 3: -8}, true},
		"three tracks, one zeroed":           {map[uint64]int64{2: 0, 3: 20, 4: 32}, true},
		"all at zero - a plain mux":          {map[uint64]int64{2: 0, 3: 0}, false},
		"all carry priming - a plain mux":    {map[uint64]int64{2: 21, 3: 20}, false},
		"single track":                       {map[uint64]int64{2: 0}, false},
		"no audio":                           {nil, false},
		"a track still above the floor":      {map[uint64]int64{2: 0, 3: 12_000}, false},
		"deliberate dub with priming sibling": {map[uint64]int64{2: 12_000, 3: 20}, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, appmaintenance.RetimeSignature(tc.delays))
		})
	}
}

// The confidence floor is CALIBRATED by the operator's own verdicts on real
// films, and sits in the gap between them: the two false positives scored
// 0.44 (a comedy) and 0.48 (a music drama), the three genuine displacements
// 0.62, 0.68 and 0.79. Lowering it would accuse the first two; raising it
// would miss the third. This test exists so that nobody "tunes" it without
// reading this.
func TestAudioCoherenceMinConfidence_SitsInTheGroundTruthGap(t *testing.T) {
	falsePositives := []float64{0.44, 0.48}
	genuine := []float64{0.62, 0.68, 0.79}
	for _, r := range falsePositives {
		assert.Less(t, r, appmaintenance.AudioCoherenceMinConfidence,
			"a measurement at r=%.2f was a false positive on a real film and must be discarded", r)
	}
	for _, r := range genuine {
		assert.GreaterOrEqual(t, r, appmaintenance.AudioCoherenceMinConfidence,
			"a measurement at r=%.2f was a genuine displacement and must be kept", r)
	}
}
