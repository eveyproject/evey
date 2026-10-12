package mkvgo

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	extmkv "github.com/gravity-zero/mkvgo/mkv"
	"github.com/gravity-zero/mkvgo/mkv/ops"
	extwriter "github.com/gravity-zero/mkvgo/mkv/writer"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/domain/mkv"
)

// genSparselyCuedMKV writes a real MKV whose video keyframes are dense (one per
// second) but whose Cues element names only one every 60 s - mkvgo's
// "index-sparse" shape: the entries are keyed on the right track, there are
// just too few of them, so a seek lands up to a minute from its target.
// WriteClusterWithCues cues every video keyframe by construction, so the
// entries are thinned afterwards to reproduce it. A reindex CAN fix this file:
// the keyframes it would cue are one second apart.
func genSparselyCuedMKV(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "sparsely_cued.mkv")
	f, err := os.Create(p)
	require.NoError(t, err)
	defer f.Close()

	mw := extwriter.NewMKVWriter(f)
	require.NoError(t, mw.WriteStart())
	c := &extmkv.Container{Info: extmkv.SegmentInfo{TimecodeScale: 1_000_000}}
	tracks := []extmkv.Track{
		{ID: 1, Type: extmkv.VideoTrack, Codec: "h264", Language: "eng", IsDefault: true},
		{ID: 2, Type: extmkv.AudioTrack, Codec: "aac", Language: "eng", IsDefault: true},
	}
	const durationMs = 120_000
	require.NoError(t, mw.WriteMetadata(c, tracks, durationMs))
	for ts := int64(0); ts <= durationMs; ts += 1000 {
		blocks := []extmkv.Block{
			{TrackNumber: 1, Timecode: ts, Keyframe: true, Data: []byte("v")},
			{TrackNumber: 2, Timecode: ts, Keyframe: true, Data: []byte("a")},
		}
		require.NoError(t, mw.WriteClusterWithCues(ts, 1_000_000, blocks))
	}
	kept := mw.Cues[:0]
	for _, cue := range mw.Cues {
		if cue.TimeMs%60_000 == 0 {
			kept = append(kept, cue)
		}
	}
	mw.Cues = kept
	require.NoError(t, mw.Finalize())

	// Guard the fixture itself: the point of this file is the sparse verdict,
	// and a writer change that re-densified the cues would silently turn the
	// regression tests below into tautologies.
	ch, err := ops.CueHealth(context.Background(), p)
	require.NoError(t, err)
	require.False(t, ch.Healthy, "fixture must be diagnosed unhealthy (sparse)")
	require.Positive(t, ch.VideoCues, "fixture must keep video-keyed cues - that is what makes it sparse rather than misskeyed")
	return p
}

// THE 2026-07-26 PROD LOOP, second act. The 2026-07-14 fix aligned the repair's
// skip check on CueHealth, but on `VideoCues > 0` rather than on the scan's
// actual verdict (`Healthy`). mkvgo v0.25.0 then added "index-sparse", whose
// definition is video cues that exist and are too far apart: VideoCues > 0
// holds BY CONSTRUCTION, so every such file is flagged by every scan and
// skipped by every repair. Prod: 138 files "repaired", 134 still flagged.
// The repair must judge with the scan's yardstick, whole.
func TestHasSeekIndex_SparseCues_False(t *testing.T) {
	a := New()
	p := genSparselyCuedMKV(t)

	has, err := a.HasSeekIndex(context.Background(), p)
	require.NoError(t, err)
	if has {
		t.Error("an index too coarse to seek into is not a usable index - reporting it as one is what pins the file in the scan/repair loop")
	}
}

func TestReindexInPlace_SparseCues_RepairsInsteadOfSkipping(t *testing.T) {
	a := New()
	p := genSparselyCuedMKV(t)

	res, err := a.ReindexInPlace(context.Background(), p, mkv.ReindexOptions{})
	require.NoError(t, err)
	if res.Skipped {
		t.Fatal("a sparse index must be rebuilt, not skipped as \"already indexed\" - this is the loop that left 134 prod files unrepaired after a 138-file run")
	}

	has, err := a.HasSeekIndex(context.Background(), p)
	require.NoError(t, err)
	require.True(t, has, "after the repair the scan's own judge must be satisfied - otherwise the loop just comes back")
}
