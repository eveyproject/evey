package mkvgo_test

// Guards for a Matroska timebase that is not the 1 ms default.
//
// These exercise EVEY'S ADAPTER, not mkvgo's internals - the library has its own
// guards; what these pin is that OUR consumers survive a non-default
// TimecodeScale. Two production files (DVDFab 9.x DVD rips at 20832 ns/unit)
// failed every repair with "reindex verify: cue 1 mismatch", their dense index
// read as unusable, and every thumbnail of an 84-minute film came from its
// first two minutes.
//
// VERIFIED against mkvgo v0.28.1 and against v0.29.0 (2026-09-07): 5 of the 6
// fail on the old pin - four with the exact production error, and the playlist
// one announcing 48.003 s segments for a 6 s target. The sixth is the control at
// the default timebase, which passes on both and is what proves the other five
// are about the timebase and not about the fixture.
// LAND THIS WITH THE PIN BUMP - it does not compile-fail on the old library, it
// behaviour-fails, which is the whole point.

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	extmkv "github.com/gravity-zero/mkvgo/mkv"
	"github.com/gravity-zero/mkvgo/mkv/writer"

	adapter "github.com/eveyproject/evey/internal/adapters/driven/mkvgo"
	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	"github.com/eveyproject/evey/internal/domain/mkv"
)

// 20832 ns per unit (~1/48000 s, 48 units to the millisecond) is the value
// measured on the two rips that exposed the defect. Reading those units as
// milliseconds makes every cue time 48x too large.
const wildScale = 20832

func u32(v uint32) *uint32   { return new(v) }
func u8(v uint8) *uint8      { return new(v) }
func f64(v float64) *float64 { return new(v) }

// buildScaled writes one video keyframe and one audio block per cluster, one
// cluster per second, with NO Cues - the index is what a repair rebuilds.
func buildScaled(t *testing.T, path string, scale int64, clusters int) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	mw := writer.NewMKVWriter(f)
	if err := mw.WriteStart(); err != nil {
		t.Fatal(err)
	}
	c := &extmkv.Container{Info: extmkv.SegmentInfo{TimecodeScale: scale, MuxingApp: "evey-test", WritingApp: "evey-test"}}
	tracks := []extmkv.Track{
		{ID: 1, Type: extmkv.VideoTrack, Codec: "h264", Language: "eng", Width: u32(1920), Height: u32(1080), CodecPrivate: []byte{0x01}},
		{ID: 2, Type: extmkv.AudioTrack, Codec: "aac", Language: "eng", SampleRate: f64(48000), Channels: u8(2)},
	}
	if err := mw.WriteMetadata(c, tracks, int64(clusters)*1000); err != nil {
		t.Fatal(err)
	}
	for i := range clusters {
		ts := int64(i) * 1000
		blocks := []extmkv.Block{
			// A minimal AVCC access unit: 4-byte length prefix + an IDR NAL. A
			// one-byte payload makes the keyframe extractor refuse with
			// "truncated NAL length", which says nothing about the timebase.
			{TrackNumber: 1, Timecode: ts, Keyframe: true, Data: []byte{0x00, 0x00, 0x00, 0x02, 0x65, 0x88}},
			{TrackNumber: 2, Timecode: ts, Keyframe: true, Data: []byte{0x01}},
		}
		if err := writer.WriteCluster(f, ts, scale, blocks); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Finalize(); err != nil {
		t.Fatal(err)
	}
}

// buildScaledWithCues is buildScaled plus an index the muxer wrote itself - the
// shape a correct reindex leaves behind. It exists so the PlanHLS probe can run
// WITHOUT a repair first: with the repair in the way, the old library fails at
// the reindex and the playlist assertions are never reached, which would prove
// nothing about the playlist.
func buildScaledWithCues(t *testing.T, path string, scale int64, clusters int) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	mw := writer.NewMKVWriter(f)
	if err := mw.WriteStart(); err != nil {
		t.Fatal(err)
	}
	c := &extmkv.Container{Info: extmkv.SegmentInfo{TimecodeScale: scale, MuxingApp: "evey-test", WritingApp: "evey-test"}}
	tracks := []extmkv.Track{
		{ID: 1, Type: extmkv.VideoTrack, Codec: "h264", Language: "eng", Width: u32(1920), Height: u32(1080), CodecPrivate: []byte{0x01}},
		{ID: 2, Type: extmkv.AudioTrack, Codec: "aac", Language: "eng", SampleRate: f64(48000), Channels: u8(2)},
	}
	if err := mw.WriteMetadata(c, tracks, int64(clusters)*1000); err != nil {
		t.Fatal(err)
	}
	for i := range clusters {
		ts := int64(i) * 1000
		blocks := []extmkv.Block{
			{TrackNumber: 1, Timecode: ts, Keyframe: true, Data: []byte{0x00, 0x00, 0x00, 0x02, 0x65, 0x88}},
			{TrackNumber: 2, Timecode: ts, Keyframe: true, Data: []byte{0x01}},
		}
		if err := mw.WriteClusterWithCues(ts, scale, blocks); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Finalize(); err != nil {
		t.Fatal(err)
	}
}

// The two production files were 84 and 162 minutes; 300 clusters keeps the
// fixture cheap while staying long enough that a 48x error puts every answer in
// the first few seconds.
const fixtureClusters = 300

// A repair must SUCCEED on a non-default timebase. The strict rewrite rebuilt
// the index correctly and then rejected its own output, because the
// verification compared raw units against milliseconds.
func TestAdapter_ReindexSucceedsOnWildTimecodeScale(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dvdfab9.mkv")
	buildScaled(t, path, wildScale, fixtureClusters)
	a := &adapter.Adapter{}

	res, err := a.ReindexInPlace(context.Background(), path, mkv.ReindexOptions{Mode: mkv.ModeRewrite, KeepBackup: true})
	if err != nil {
		t.Fatalf("repair refused a file it had just rebuilt correctly: %v", err)
	}
	if res.Skipped {
		t.Fatal("repair skipped a file with no index at all")
	}
}

// HasSeekIndex IS the scan's HasIndex verdict. A dense index on a non-default
// timebase read as unhealthy, which queued a perfectly indexed file for repair
// and kept it there, run after run.
func TestAdapter_DenseIndexIsHealthyOnWildTimecodeScale(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dvdfab9.mkv")
	buildScaled(t, path, wildScale, fixtureClusters)
	a := &adapter.Adapter{}
	ctx := context.Background()

	if _, err := a.ReindexInPlace(ctx, path, mkv.ReindexOptions{Mode: mkv.ModeRewrite}); err != nil {
		t.Fatalf("reindex: %v", err)
	}
	ok, err := a.HasSeekIndex(ctx, path)
	if err != nil {
		t.Fatalf("has-seek-index: %v", err)
	}
	if !ok {
		t.Error("an index with one cue per second reads as unusable - the scan would queue this file for a repair it does not need")
	}
}

// The verdict the media-health page shows. "index-sparse" on a densely indexed
// file is the false positive that pins files in a sterile loop.
func TestAdapter_DiagnoseIsCleanOnWildTimecodeScale(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dvdfab9.mkv")
	buildScaled(t, path, wildScale, fixtureClusters)
	a := &adapter.Adapter{}
	ctx := context.Background()

	if _, err := a.ReindexInPlace(ctx, path, mkv.ReindexOptions{Mode: mkv.ModeRewrite}); err != nil {
		t.Fatalf("reindex: %v", err)
	}
	d, err := a.Diagnose(ctx, path)
	if err != nil {
		t.Fatalf("diagnose: %v", err)
	}
	if d.HasIndexDefect() {
		t.Errorf("index defect reported on a rebuilt dense index: %+v", d.Findings)
	}
	if d.IndexSparse() {
		t.Errorf("index-sparse on one cue per second: %q", d.IndexSparseDetail())
	}
}

// The consumer with a visible output: asking for a frame at T must land near T,
// not at T/48. Every thumbnail of an 84-minute film came from its first ~1.7
// minutes, and a storyboard was a row of near-identical opening frames.
func TestAdapter_KeyframeExtractionLandsNearTheRequestedTime(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dvdfab9.mkv")
	buildScaled(t, path, wildScale, fixtureClusters)
	a := &adapter.Adapter{}
	ctx := context.Background()
	// Extraction seeks THROUGH the Cues, so the index has to exist first - the
	// control below does the same, and skipping it made this test fail for a
	// reason that had nothing to do with the timebase.
	if _, err := a.ReindexInPlace(ctx, path, mkv.ReindexOptions{Mode: mkv.ModeRewrite}); err != nil {
		t.Fatalf("reindex: %v", err)
	}

	const askMs = 240_000 // 4 minutes in
	_, _, gotMs, err := a.ExtractKeyframePacket(ctx, path, askMs)
	if err != nil {
		t.Fatalf("extract keyframe: %v", err)
	}
	// Keyframes sit one second apart, so the nearest is within a second either
	// way. A 48x error answers around 5 000 ms.
	if gotMs < askMs-1500 || gotMs > askMs+1500 {
		t.Errorf("asked for a frame at %d ms, got the one at %d ms - a thumbnail would come from the wrong scene", askMs, gotMs)
	}
}

// Control, and it earns its place: the same assertions at the DEFAULT timebase,
// which must pass on both the old and the new library. Without it a before/after
// proves the fixture changed behaviour, not that the fix did - and it caught two
// fixture faults of mine (a one-byte video payload, then a missing reindex)
// that would otherwise have read as verdicts on the fix.
func TestAdapter_DefaultTimecodeScaleStillWorks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ordinary.mkv")
	buildScaled(t, path, 1_000_000, fixtureClusters)
	a := &adapter.Adapter{}
	ctx := context.Background()

	if _, err := a.ReindexInPlace(ctx, path, mkv.ReindexOptions{Mode: mkv.ModeRewrite}); err != nil {
		t.Fatalf("reindex: %v", err)
	}
	ok, err := a.HasSeekIndex(ctx, path)
	if err != nil {
		t.Fatalf("has-seek-index: %v", err)
	}
	if !ok {
		t.Error("dense index unhealthy at the default scale - the fixture itself is wrong")
	}
	_, _, gotMs, err := a.ExtractKeyframePacket(ctx, path, 240_000)
	if err != nil {
		t.Fatalf("extract keyframe: %v", err)
	}
	if gotMs < 238_500 || gotMs > 241_500 {
		t.Errorf("keyframe at %d ms for a 240000 ms request at the default scale", gotMs)
	}
}

// The one consumer with a VIEWER behind it: the copy rung serves HLS straight
// from mp4.PlanHLS. On a non-default timebase the segmentation degenerated to
// one segment per cluster and every announced duration came out 48x the truth,
// so a player built its timeline, buffer and seek maths on fiction.
func TestAdapter_OnDemandPlaylistIsSaneOnWildTimecodeScale(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dvdfab9.mkv")
	// Indexed by the muxer, NOT repaired first: a repair in the way makes the
	// pre-fix library fail before the playlist is ever built.
	buildScaledWithCues(t, path, wildScale, fixtureClusters)
	ctx := context.Background()

	pk := adapter.NewOnDemandPackager()
	defer pk.Close()
	plan, err := pk.Plan(ctx, path, apptranscoding.OnDemandOptions{SegmentMs: 6000})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	// 300 s of media at a 6 s target: ~50 segments. One per cluster would be 300.
	if n := plan.NumSegments(); n > 80 {
		t.Errorf("%d segments for %d s of media at a 6 s target - the segment grid collapsed onto the cluster grid", n, fixtureClusters)
	}
	pl, _, err := plan.Resource(ctx, "playlist.m3u8")
	if err != nil {
		t.Fatalf("playlist: %v", err)
	}
	total := 0.0
	for line := range strings.SplitSeq(string(pl), "\n") {
		if !strings.HasPrefix(line, "#EXTINF:") {
			continue
		}
		v, perr := strconv.ParseFloat(strings.TrimSuffix(strings.TrimPrefix(line, "#EXTINF:"), ","), 64)
		if perr != nil {
			t.Fatalf("unparseable EXTINF %q: %v", line, perr)
		}
		if v > 30 {
			t.Errorf("a segment announces %.3f s for a 6 s target - durations are scaling with the timebase", v)
		}
		total += v
	}
	// The real media is 300 s. A 48x error advertises ~14 400 s.
	if total < 280 || total > 320 {
		t.Errorf("playlist announces %.1f s of media for a %d s file", total, fixtureClusters)
	}
}

// The operator's shift, on a fine timebase. Two limits used to stack here, and
// only ONE is gone.
//
// Fixed in mkvgo v0.29.1: a retime rounded the request to the nearest timecode
// unit and then refused it unless the rounding had been a no-op, so on a
// 20832 ns/unit file the only applicable shift was 651 ms - while our
// sync-preview panel offers a 0.05 s slider. Any in-range value applies now.
//
// NOT fixed, and it bites the same panel: a block's offset from its cluster is
// a SIGNED 16-BIT count of timecode units, so at 20832 ns the whole range is
// about +-682 ms. Our panel offers +-60 s. On such a file everything past
// ~0.68 s is still refused, and the operator only finds out on Apply - which is
// why that refusal has to stay readable on screen.
func TestAdapter_OperatorShiftAppliesOnWildTimecodeScale(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dvdfab9.mkv")
	buildScaledWithCues(t, path, wildScale, 30)

	// +400 ms: not a whole number of ticks at 20832 ns, and not one of the
	// multiples the old rounding rule allowed - the exact shape that used to be
	// refused, and comfortably inside the int16 range. Forwards, because this
	// fixture's tracks start at 0 and pulling one BACK would make the first
	// blocks land before the start of the file - a refusal about the fixture,
	// not about the timebase.
	if err := (&adapter.TrackRetimer{}).RetimeTracks(context.Background(), path, map[uint64]float64{2: 0.4}); err != nil {
		t.Fatalf("an in-range offset an operator could pick was refused: %v", err)
	}
}

// The limit that remains, pinned so nobody reads the test above as "any shift
// works now". If this ever starts passing, mkvgo re-clusters and the panel can
// stop bounding itself.
func TestAdapter_OperatorShiftBeyondInt16IsStillRefused(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dvdfab9.mkv")
	buildScaledWithCues(t, path, wildScale, 30)

	err := (&adapter.TrackRetimer{}).RetimeTracks(context.Background(), path, map[uint64]float64{2: 2.4})
	if err == nil {
		t.Fatal("a 2.4 s shift applied on a 20832 ns file - the int16 cluster-offset limit is gone, update the panel's bounds")
	}
	// The operator reads this sentence in the panel, so it has to say what is
	// wrong rather than just refusing.
	if !strings.Contains(err.Error(), "representable timecode range") {
		t.Errorf("refusal does not explain itself: %v", err)
	}
}
