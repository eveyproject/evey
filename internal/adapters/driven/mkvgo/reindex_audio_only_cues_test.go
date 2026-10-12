package mkvgo

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	extmkv "github.com/gravity-zero/mkvgo/mkv"
	extwriter "github.com/gravity-zero/mkvgo/mkv/writer"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/domain/mkv"
)

// genAudioOnlyCuedMKV writes a real MKV whose Cues element exists and is
// non-empty but keys every entry onto the audio track, never the video one -
// the same real-world muxing shape mkvgo's CueHealth calls "index-misskeyed":
// a video seek can never land on a keyframe because the index doesn't name
// one. WriteClusterWithCues cues the video track by construction, so the
// entries are retargeted onto the audio track afterwards to reproduce it.
func genAudioOnlyCuedMKV(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "audio_only_cued.mkv")
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
	require.NoError(t, mw.WriteMetadata(c, tracks, 3000))
	for _, ts := range []int64{0, 1000, 2000} {
		blocks := []extmkv.Block{
			{TrackNumber: 1, Timecode: ts, Keyframe: true, Data: []byte("v")},
			{TrackNumber: 2, Timecode: ts, Keyframe: true, Data: []byte("a")},
		}
		require.NoError(t, mw.WriteClusterWithCues(ts, 1_000_000, blocks))
	}
	for i := range mw.Cues {
		mw.Cues[i].Track = 2
	}
	require.NoError(t, mw.Finalize())
	return p
}

// A Cues element that exists, is non-empty, and keys onto the audio track
// only cannot serve a video seek at all - every seek lands mid-GOP. Reporting
// it as "already has a seek index" (the naive len(Cues) > 0 check) means the
// scan never offers to reindex it. This is the same defect class CueHealth's
// misskeyed finding exists for, reintroduced independently here because
// HasSeekIndex redid its own raw Cues-count check instead of judging the
// video-keyed cues the way the rest of mkvgo does.
func TestHasSeekIndex_AudioOnlyCues_False(t *testing.T) {
	a := New()
	p := genAudioOnlyCuedMKV(t)

	has, err := a.HasSeekIndex(context.Background(), p)
	require.NoError(t, err)
	if has {
		t.Error("an index keyed only on the audio track cannot serve a video seek - must not be reported as already indexed")
	}
}

// THE 2026-07-14 PROD LOOP. The scan flagged 140 files "no usable index"; the
// repair batch then skipped 129 of them as "already indexed" - and the panel's
// count never moved, run after run. Same file, two judges: the scan asks
// CueHealth "can a video seek USE this index?", the repair's skip check only
// counted raw Cues entries. A phantom index (empty, audio-keyed, or not
// reachable through the SeekHead) passes the second and fails the first,
// forever. The repair must judge with the scan's yardstick: skip only what the
// scan itself would call indexed.
func TestReindexInPlace_AudioOnlyCues_RepairsInsteadOfSkipping(t *testing.T) {
	a := New()
	p := genAudioOnlyCuedMKV(t)

	res, err := a.ReindexInPlace(context.Background(), p, mkv.ReindexOptions{})
	require.NoError(t, err)
	if res.Skipped {
		t.Fatal("an index no video seek can use must be rebuilt, not skipped as \"already indexed\" - this is the scan/repair loop that pinned 129 files")
	}

	has, err := a.HasSeekIndex(context.Background(), p)
	require.NoError(t, err)
	require.True(t, has, "after the repair, the scan's own judge must be satisfied - otherwise the loop just comes back")
}
