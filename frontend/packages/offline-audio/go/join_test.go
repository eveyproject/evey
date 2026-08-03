package offlineaudio

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/gravity-zero/mkvgo/mkv"
	"github.com/gravity-zero/mkvgo/mkv/reader"
	"github.com/gravity-zero/mkvgo/mkv/writer"
)

// writeSyntheticSlice builds a minimal but real Matroska slice: one video
// track, blocks every 40 ms starting at zero (the server's slices are
// timestamp-reset, which is what the join has to re-offset).
func writeSyntheticSlice(t *testing.T, path string, blocks int) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()

	w := uint32(1152)
	h := uint32(720)
	track := mkv.Track{
		ID:                1,
		Type:              mkv.VideoTrack,
		Codec:             "V_MPEG4/ISO/AVC",
		CodecPrivate:      []byte{0x01, 0x64, 0x00, 0x28}, // identical across slices: one encoder
		Width:             &w,
		Height:            &h,
		DefaultDurationNs: 40_000_000,
	}
	c := &mkv.Container{
		Info:   mkv.SegmentInfo{TimecodeScale: 1_000_000, MuxingApp: "test", WritingApp: "test"},
		Tracks: []mkv.Track{track},
	}
	mw := writer.NewMKVWriter(f)
	if err := mw.WriteStart(); err != nil {
		t.Fatalf("WriteStart: %v", err)
	}
	if err := mw.WriteMetadata(c, c.Tracks, int64(blocks)*40); err != nil {
		t.Fatalf("WriteMetadata: %v", err)
	}
	var batch []mkv.Block
	for i := range blocks {
		batch = append(batch, mkv.Block{
			TrackNumber: 1,
			Timecode:    int64(i) * 40,
			Keyframe:    i == 0,
			Data:        []byte{byte(i), 0x02, 0x03},
		})
	}
	if err := mw.WriteClusterWithCues(0, 1_000_000, batch); err != nil {
		t.Fatalf("WriteClusterWithCues: %v", err)
	}
	if err := mw.Finalize(); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
}

func TestJoinSlicesDeclaresTheRealDuration(t *testing.T) {
	dir := t.TempDir()
	// Three slices of 10 blocks (400 ms each): 1200 ms of film. Each slice
	// declares 400 ms of its own, which was exactly the v0.25.1 trap - Join
	// copied the FIRST slice's duration into the joined header (a 400 ms
	// film that refuses to seek). Since v0.26.0 the computed total wins.
	const slices, blocksPer, totalMs = 3, 10, 1200
	for i := range slices {
		writeSyntheticSlice(t, filepath.Join(dir, SliceName(i)), blocksPer)
	}

	out := filepath.Join(dir, "film.mkv")
	if err := JoinSlices(dir, slices, out, totalMs); err != nil {
		t.Fatalf("JoinSlices: %v", err)
	}

	c, err := reader.Open(context.Background(), out)
	if err != nil {
		t.Fatalf("reopen the joined file: %v", err)
	}
	if math.Abs(float64(c.DurationMs)-totalMs) > 1 {
		t.Errorf("joined file declares %d ms, want %d ms - mkvgo's Join lost the total", c.DurationMs, totalMs)
	}

	// And the content really is all three slices, re-offset onto one timeline.
	f, err := os.Open(out)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = f.Close() }()
	br, err := reader.NewBlockReader(f, c.Info.TimecodeScale)
	if err != nil {
		t.Fatalf("NewBlockReader: %v", err)
	}
	var n int
	var last int64
	for {
		b, err := br.Next()
		if err != nil {
			break
		}
		if b.Timecode < last {
			t.Fatalf("block %d goes backwards in time (%d after %d) - the slices were not re-offset", n, b.Timecode, last)
		}
		last = b.Timecode
		n++
	}
	if n != slices*blocksPer {
		t.Errorf("joined file holds %d blocks, want %d - a slice was dropped", n, slices*blocksPer)
	}
	if last < totalMs-100 {
		t.Errorf("last block at %d ms, want near %d ms", last, totalMs)
	}

	// The slices survive the join on purpose: they are the only thing between
	// a crash and re-downloading the film. They go when the caller says so.
	for i := range slices {
		if _, err := os.Stat(filepath.Join(dir, SliceName(i))); err != nil {
			t.Errorf("slice %d was removed by the join itself: %v", i, err)
		}
	}
	DeleteSlices(dir, slices)
	for i := range slices {
		if _, err := os.Stat(filepath.Join(dir, SliceName(i))); !os.IsNotExist(err) {
			t.Errorf("slice %d survived DeleteSlices", i)
		}
	}
}

// A missing slice must be refused up front, with the index named. Half a film
// silently joined into a "finished" download is the worst possible outcome.
func TestJoinSlicesRefusesAGap(t *testing.T) {
	dir := t.TempDir()
	writeSyntheticSlice(t, filepath.Join(dir, SliceName(0)), 5)
	writeSyntheticSlice(t, filepath.Join(dir, SliceName(2)), 5)

	out := filepath.Join(dir, "film.mkv")
	err := JoinSlices(dir, 3, out, 600)
	if err == nil {
		t.Fatal("JoinSlices accepted a gap in the slices")
	}
	if got := err.Error(); got[:len(errSliceMissing)] != errSliceMissing {
		t.Errorf("error %q does not start with the %q code the JS side routes on", got, errSliceMissing)
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Error("a refused join left an output file behind - it would look like a finished download")
	}
}

func TestJoinSlicesRefusesAnEmptySet(t *testing.T) {
	if err := JoinSlices(t.TempDir(), 0, filepath.Join(t.TempDir(), "x.mkv"), 0); err == nil {
		t.Fatal("JoinSlices accepted zero slices")
	}
}

// A failed join must not leave the ".joining" scratch file behind: the next
// attempt writes to the same path, and a stale one would be silently reused.
func TestJoinSlicesCleansUpAfterAFailure(t *testing.T) {
	dir := t.TempDir()
	writeSyntheticSlice(t, filepath.Join(dir, SliceName(0)), 5)
	// A slice that is not Matroska at all: Join fails partway, after creating
	// the output.
	if err := os.WriteFile(filepath.Join(dir, SliceName(1)), []byte("not an mkv"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "film.mkv")
	if err := JoinSlices(dir, 2, out, 400); err == nil {
		t.Fatal("JoinSlices accepted a corrupt slice")
	}
	if _, err := os.Stat(out + ".joining"); !os.IsNotExist(err) {
		t.Error("the scratch file survived a failed join")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Error("a failed join left an output file behind")
	}
}
