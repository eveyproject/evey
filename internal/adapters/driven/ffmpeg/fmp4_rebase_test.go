package ffmpeg

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/evey-project/evey/internal/domain/transcoding"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/evey-project/evey/internal/pkg/metrics"
)

func be32(v uint32) []byte { b := make([]byte, 4); binary.BigEndian.PutUint32(b, v); return b }
func be64(v uint64) []byte { b := make([]byte, 8); binary.BigEndian.PutUint64(b, v); return b }

func box(typ string, payload ...[]byte) []byte {
	body := bytes.Join(payload, nil)
	out := make([]byte, 0, 8+len(body))
	out = append(out, be32(uint32(8+len(body)))...)
	out = append(out, typ...)
	return append(out, body...)
}

// fullbox prepends version+flags to the payload.
func fullbox(typ string, ver byte, payload ...[]byte) []byte {
	vf := []byte{ver, 0, 0, 0}
	return box(typ, append([][]byte{vf}, payload...)...)
}

func writeTmp(t *testing.T, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "seg.m4s")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func synthSegment(tfdtVer byte, tfdtVal uint64) []byte {
	sidx := fullbox("sidx", 0,
		be32(1),    // reference_ID
		be32(1000), // timescale
		be32(5000), // earliest_presentation_time (v0)
		be32(0),    // first_offset
		be32(0),    // reserved(2)+reference_count(2)
	)
	tfhd := fullbox("tfhd", 0, be32(1)) // track_ID 1
	var tfdt []byte
	if tfdtVer == 1 {
		tfdt = fullbox("tfdt", 1, be64(tfdtVal))
	} else {
		tfdt = fullbox("tfdt", 0, be32(uint32(tfdtVal)))
	}
	moof := box("moof", box("traf", tfhd, tfdt))
	return bytes.Join([][]byte{box("styp", []byte("msdh")), sidx, moof, box("mdat")}, nil)
}

func readSidxEPT(t *testing.T, p string) uint32 {
	t.Helper()
	buf, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	i := bytes.Index(buf, []byte("sidx"))
	return binary.BigEndian.Uint32(buf[i+16 : i+20]) // +4 verflags +4 refID +4 ts
}

func readTfdt(t *testing.T, p string) uint64 {
	t.Helper()
	buf, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	i := bytes.Index(buf, []byte("tfdt"))
	if buf[i+4] == 1 {
		return binary.BigEndian.Uint64(buf[i+8 : i+16])
	}
	return uint64(binary.BigEndian.Uint32(buf[i+8 : i+12]))
}

func TestRebaseShiftsTfdtAndSidx(t *testing.T) {
	p := writeTmp(t, synthSegment(1, 5000))
	// 2500 ms at timescale 1000 → +2500 units
	tfdtMs, err := rebaseFMP4Segment(p, 2500, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := readSidxEPT(t, p); got != 7500 {
		t.Fatalf("sidx EPT = %d, want 7500", got)
	}
	if got := readTfdt(t, p); got != 7500 {
		t.Fatalf("tfdt = %d, want 7500", got)
	}
	// timescale 1000 → 7500 units = 7500 ms; the pump's tripwire consumes this
	if tfdtMs != 7500 {
		t.Fatalf("returned first tfdt = %dms, want 7500", tfdtMs)
	}
}

// A late flush failure only ever surfaces on Close (ENOSPC, a dying NFS
// share); the WriteAt calls above it can report success into the page cache
// regardless. rebaseFMP4Segment must turn that into a real error and a
// withheld tfdt, or the pump's sweep would rename a truncated segment as if
// it were valid.
func TestRebaseFMP4Segment_CloseErrorWithholdsSegment(t *testing.T) {
	p := writeTmp(t, synthSegment(1, 5000))

	orig := closeSegmentFile
	closeSegmentFile = func(f *os.File) error {
		_ = f.Close() // still release the real fd; only report the failure
		return errors.New("simulated flush failure")
	}
	t.Cleanup(func() { closeSegmentFile = orig })

	tfdtMs, err := rebaseFMP4Segment(p, 2500, nil)
	if err == nil {
		t.Fatal("want an error when Close fails - a flush failure must not look like success")
	}
	if tfdtMs != -1 {
		t.Fatalf("tfdtMs = %d, want -1: a Close failure must not hand back a value the caller could publish", tfdtMs)
	}
}

// End-to-end through the pump's own publish gate: a segment whose Close
// failed must never appear in pubDir. Before the fix, rebaseFMP4Segment
// swallowed the Close error, so the pump's `if err != nil { continue }` guard
// never triggered and os.Rename published the truncated segment as valid.
func TestRunRebasePump_CloseErrorNeverPublishesSegment(t *testing.T) {
	priv := t.TempDir()
	pub := t.TempDir()
	segName := "stream_0_0.m4s"
	if err := os.WriteFile(filepath.Join(priv, segName), synthSegment(1, 5000), 0o644); err != nil {
		t.Fatal(err)
	}

	orig := closeSegmentFile
	closeSegmentFile = func(f *os.File) error {
		_ = f.Close()
		return errors.New("simulated flush failure")
	}
	t.Cleanup(func() { closeSegmentFile = orig })

	stop := make(chan struct{})
	close(stop) // drives the pump straight to its final sweep, no ticker wait
	runRebasePump(nil, stop, priv, pub, 2500, "stream", 0, nil, "copy")

	if _, err := os.Stat(filepath.Join(pub, segName)); err == nil {
		t.Fatal("segment must NOT be published when its Close failed - the client would receive corrupted bytes")
	}
}

func TestRebaseV0OverflowLeavesFileUntouched(t *testing.T) {
	orig := synthSegment(0, 0xFFFFFF00)
	p := writeTmp(t, orig)
	_, err := rebaseFMP4Segment(p, 2500, nil)
	if err == nil {
		t.Fatal("want overflow error")
	}
	after, _ := os.ReadFile(p)
	if !bytes.Equal(orig, after) {
		t.Fatal("file modified despite error (compute-first-write-last broken)")
	}
}

func TestRebaseInitFallbackWhenNoSidx(t *testing.T) {
	// segment without sidx: timescale must come from the init map
	tfhd := fullbox("tfhd", 0, be32(7))
	tfdt := fullbox("tfdt", 1, be64(1000))
	seg := bytes.Join([][]byte{box("styp", []byte("msdh")), box("moof", box("traf", tfhd, tfdt)), box("mdat")}, nil)
	p := writeTmp(t, seg)

	if _, err := rebaseFMP4Segment(p, 1000, nil); err == nil {
		t.Fatal("want error without any timescale source")
	}
	if _, err := rebaseFMP4Segment(p, 1000, map[uint32]uint32{7: 90000}); err != nil {
		t.Fatal(err)
	}
	if got := readTfdt(t, p); got != 1000+90000 {
		t.Fatalf("tfdt = %d, want %d", got, 1000+90000)
	}
}

func TestParseInitTimescales(t *testing.T) {
	tkhd := fullbox("tkhd", 0, be32(0), be32(0), be32(3)) // creation, modification, track_ID
	mdhd := fullbox("mdhd", 0, be32(0), be32(0), be32(48000), be32(0))
	moov := box("moov", box("trak", tkhd, box("mdia", mdhd)))
	p := writeTmp(t, bytes.Join([][]byte{box("ftyp", []byte("iso6")), moov}, nil))

	ts, err := parseInitTimescales(p)
	if err != nil {
		t.Fatal(err)
	}
	if ts[3] != 48000 {
		t.Fatalf("timescale track3 = %d, want 48000", ts[3])
	}
}

func TestFmp4CompleteMoov(t *testing.T) {
	moov := box("moov", box("trak"))
	full := bytes.Join([][]byte{box("ftyp", []byte("iso6")), moov}, nil)
	p := writeTmp(t, full)
	if !fmp4CompleteMoov(p) {
		t.Fatal("complete init reported incomplete")
	}
	if err := os.WriteFile(p, full[:len(full)-5], 0o644); err != nil {
		t.Fatal(err)
	}
	if fmp4CompleteMoov(p) {
		t.Fatal("truncated init reported complete")
	}
}

// The pump tripwire: a published copy segment whose timeline diverges from
// the plan beyond tolerance must trip the metric/ERROR; aligned segments,
// unknown indexes and non-matching names must not.
func TestVerifyCopySegmentTimeline(t *testing.T) {
	plan := func(idx int) (int64, bool) {
		if idx == 600 {
			return 1351308, true
		}
		return 0, false
	}
	read := func() float64 { return testutil.ToFloat64(metrics.HLSVODCopyTimelineDrift) }

	base := read()
	r := &timelineDriftReporter{}
	// Aligned (within tolerance: nudge + priming scale skews).
	r.verify("stream_0_600.m4s", "stream_0_", 1351284, plan, "copy")
	if read() != base {
		t.Fatal("aligned segment must not trip the drift tripwire")
	}
	// Unknown index / foreign name / missing tfdt: no-ops.
	r.verify("stream_0_601.m4s", "stream_0_", 999999, plan, "copy")
	r.verify("other_600.m4s", "stream_0_", 0, plan, "copy")
	r.verify("stream_0_600.m4s", "stream_0_", -1, plan, "copy")
	if read() != base {
		t.Fatal("unknown/foreign/empty segments must not trip the tripwire")
	}
	if r.seen != 0 {
		t.Fatalf("no divergence must have been recorded, got %d", r.seen)
	}
	// A GOP-sized divergence (the historical bug class) must trip it.
	r.verify("stream_0_600.m4s", "stream_0_", 1351308+6340, plan, "copy")
	if read() != base+1 {
		t.Fatal("a 6.3s divergence must increment the drift counter")
	}
	if r.seen != 1 {
		t.Fatalf("first divergence must be counted once, got %d", r.seen)
	}
}

// A broken contract diverges on EVERY segment of a session. The METRIC must
// still count each one (that is the volume signal), but the ERROR log must fire
// only once per pump: the leading-keyframe plan bug alone wrote 1238 of the
// 1239 ERRORs of a 15-day prod window and buried the only other one.
func TestVerifyCopySegmentTimeline_RepeatsDoNotFloodTheErrorLog(t *testing.T) {
	plan := func(idx int) (int64, bool) { return int64(idx) * 6000, true }
	read := func() float64 { return testutil.ToFloat64(metrics.HLSVODCopyTimelineDrift) }

	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	base := read()
	r := &timelineDriftReporter{}
	const diverged = 25
	for i := range diverged {
		r.verify(fmt.Sprintf("stream_0_%03d.m4s", i), "stream_0_", int64(i)*6000+6000, plan, "copy")
	}
	if got := read() - base; got != diverged {
		t.Fatalf("every divergence must bump the metric: got %v, want %d", got, diverged)
	}
	if r.seen != diverged {
		t.Fatalf("reporter must count every divergence, got %d want %d", r.seen, diverged)
	}
	if n := strings.Count(logs.String(), "level=ERROR"); n != 1 {
		t.Fatalf("a session-wide divergence must log ERROR exactly once, got %d", n)
	}
	// The volume stays visible: one Warn summary when the pump exits.
	r.summarize("/tmp/.rebase_0_1")
	if n := strings.Count(logs.String(), "diverged_segments=25"); n != 1 {
		t.Fatalf("the exit summary must report the total once, got %d", n)
	}
}

// The transcode-seek kind bumps a DISTINCT counter (so an alert points at the
// right pipeline) against a linear oracle: segment idx expected at idx×segLen.
func TestVerifySegmentTimeline_TranscodeSeek(t *testing.T) {
	const segLenMs = 6000
	linear := func(idx int) (int64, bool) {
		if idx < 0 {
			return 0, false
		}
		return int64(idx) * segLenMs, true
	}
	copyRead := func() float64 { return testutil.ToFloat64(metrics.HLSVODCopyTimelineDrift) }
	seekRead := func() float64 { return testutil.ToFloat64(metrics.HLSVODTranscodeSeekTimelineDrift) }

	copyBase, seekBase := copyRead(), seekRead()
	r := &timelineDriftReporter{}
	// Aligned segment 100 -> 600000 ms, within tolerance: no trip.
	r.verify("stream_0_100.m4s", "stream_0_", 600000, linear, "transcode_seek")
	if seekRead() != seekBase {
		t.Fatal("aligned transcode-seek segment must not trip")
	}
	// A GOP-sized drift bumps the TRANSCODE-SEEK counter, not the copy one.
	r.verify("stream_0_100.m4s", "stream_0_", 600000+2000, linear, "transcode_seek")
	if seekRead() != seekBase+1 {
		t.Fatal("a transcode-seek divergence must increment the transcode-seek counter")
	}
	if copyRead() != copyBase {
		t.Fatal("transcode-seek drift must NOT touch the copy counter")
	}
}

// A SEEKED audio-only session must rebase its fMP4 timestamps exactly like the
// copy-seek video path does. movenc always writes tfdt session-relative, so a
// `-ss 1800 -start_number 600` audio session emits tfdt=0 on its first fragment:
// hls.js would then place 30-minutes-in audio at the very start of the timeline,
// against a video segment correctly sitting at 1800 s - silent A/V desync.
func TestNeedsAudioFMP4Rebase(t *testing.T) {
	cold := transcoding.AudioProcessOpts{StartNumber: 0, StartSec: 0}
	if needsAudioFMP4Rebase(cold) {
		t.Error("a cold audio session (start 0) needs no rebase - tfdt is already absolute")
	}
	seeked := transcoding.AudioProcessOpts{StartNumber: 600, StartSec: 1800}
	if !needsAudioFMP4Rebase(seeked) {
		t.Error("a seeked audio session MUST be rebased, or its audio lands at t=0")
	}
}
