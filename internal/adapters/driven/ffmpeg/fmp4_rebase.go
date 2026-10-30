package ffmpeg

// Copy-seek fMP4 sessions and the shared segment cache disagree on time bases.
//
// movenc writes every fragment's tfdt (baseMediaDecodeTime) as
// `dts − start_dts(session)` - session-relative, always. No CLI flag changes
// that: `-copyts`, `-output_ts_offset` and `-avoid_negative_ts disabled` all
// still produce tfdt=0 on the first fragment (verified empirically on stock
// ffmpeg 6.1; jellyfin-ffmpeg carries no hlsenc/movenc timestamp patch, so
// prod behaves identically). A cold session (start 0) therefore emits
// absolute-looking tfdt while a `-ss`+`-start_number K` session emits 0-based
// tfdt under the SAME cache filenames - violating the SegmentCache invariant
// that one key maps to one byte-content, and feeding hls.js fragments whose
// internal timestamps contradict the playlist position (single initPTS per
// discontinuity → silent misplacement, A/V holes, stall loop after seeks).
//
// Fix: such sessions write into a private subdir; runRebasePump shifts every
// tfdt and sidx earliest_presentation_time by the session's absolute start
// (the copy plan's StartMs), then atomically renames into the cache dir. The
// rename is the publish point SegmentCache.IsReady already keys on, so no
// reader can observe a pre-rebase file.

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/eveyproject/evey/internal/pkg/metrics"
)

// closeSegmentFile applies the segment handle's final Close, as a var (not a
// bare f.Close()) so a test can force the late-flush failure Close alone can
// surface (ENOSPC, a dying NFS share) - the only way to exercise it
// deterministically, since a real OS will not fail Close() to order on a
// plain local file.
var closeSegmentFile = func(f *os.File) error { return f.Close() }

// scaleDelta converts deltaUs (microseconds) into timescale units, rounding
// to nearest. Microseconds, not milliseconds: a transcode-seek session lands
// on a source FRAME (k × 1001/24000 s = k × 41708.33 µs), and a delta rounded
// to the millisecond would leave every seek rafale up to 0.5 ms off the cold
// rafale's grid at the same timescale (24000: 12 ticks). At µs the rounding
// error is under one tick for every timescale in use (≤ 90000).
func scaleDelta(deltaUs int64, timescale uint32) int64 {
	return (deltaUs*int64(timescale) + 500_000) / 1_000_000
}

// boxHeader reads the box header at off. Returns the box type, total box size
// and an error. size==0 ("to end of file") and 64-bit largesize are handled.
func boxHeader(r io.ReaderAt, off, fileSize int64) (typ string, size int64, err error) {
	var hdr [16]byte
	if _, err = r.ReadAt(hdr[:8], off); err != nil {
		return "", 0, err
	}
	size = int64(binary.BigEndian.Uint32(hdr[0:4]))
	typ = string(hdr[4:8])
	switch size {
	case 0:
		size = fileSize - off
	case 1:
		if _, err = r.ReadAt(hdr[8:16], off+8); err != nil {
			return "", 0, err
		}
		size = int64(binary.BigEndian.Uint64(hdr[8:16]))
	}
	if size < 8 || off+size > fileSize {
		return "", 0, fmt.Errorf("fmp4: malformed box %q at %d (size %d, file %d)", typ, off, size, fileSize)
	}
	return typ, size, nil
}

// childBoxes iterates the children of the box payload buf[start:end] (offsets
// relative to buf), invoking fn(typ, off, size). Stops on malformed sizes.
func childBoxes(buf []byte, start, end int, fn func(typ string, off, size int) bool) {
	off := start
	for off+8 <= end {
		size := int(binary.BigEndian.Uint32(buf[off : off+4]))
		typ := string(buf[off+4 : off+8])
		if size == 1 && off+16 <= end {
			size = int(binary.BigEndian.Uint64(buf[off+8 : off+16]))
		}
		if size < 8 || off+size > end {
			return
		}
		if !fn(typ, off, size) {
			return
		}
		off += size
	}
}

// rebaseFMP4Segment shifts every tfdt baseMediaDecodeTime and every sidx
// earliest_presentation_time in the segment at path forward by deltaUs
// (microseconds), in place. Per-track timescales come from the segment's own sidx boxes
// (hlsenc writes one per track); initTimescales is the fallback map
// (track_ID → timescale, parsed from the session's init segment) for the
// hypothetical sidx-less layout. Patches are computed first, written last:
// any parse error leaves the file untouched. deltaUs == 0 is parse-only
// (verification pass - nothing written).
//
// Returns the FIRST traf's post-rebase tfdt in milliseconds (hlsenc writes
// the video traf first), or -1 when the segment has none - the publish
// pump's drift tripwire compares it against the keyframe plan.
func rebaseFMP4Segment(path string, deltaUs int64, initTimescales map[uint32]uint32) (result int64, err error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return -1, err
	}
	// Close is checked, not fire-and-forget deferred: the WriteAt loop below
	// can succeed into the page cache while a LATE flush failure only surfaces
	// on Close. Missing that would let the pump's sweep rename a segment whose
	// tail never reached disk - a TRUNCATED .m4s published as if it were valid,
	// corrupting the stream with no error anywhere. Same class of bug already
	// fixed once for avatar uploads (avatar_handler.go): check Close(), don't
	// discard it.
	defer func() {
		if cerr := closeSegmentFile(f); cerr != nil {
			if err == nil {
				err = fmt.Errorf("fmp4: close after rebase write: %w", cerr)
			}
			result = -1 // withhold: the caller must not treat this as a valid tfdt
		}
	}()
	fi, err := f.Stat()
	if err != nil {
		return -1, err
	}
	fileSize := fi.Size()

	type patch struct {
		off   int64
		width int // 4 or 8 bytes
		val   uint64
	}
	var patches []patch
	firstTfdtMs := int64(-1)
	tsByTrack := make(map[uint32]uint32, 2)
	maps.Copy(tsByTrack, initTimescales)

	var off int64
	for off < fileSize {
		typ, size, err := boxHeader(f, off, fileSize)
		if err != nil {
			return -1, err
		}
		switch typ {
		case "sidx":
			buf := make([]byte, size)
			if _, err := f.ReadAt(buf, off); err != nil {
				return -1, err
			}
			if size < 28 {
				return -1, fmt.Errorf("fmp4: sidx too short (%d) at %d", size, off)
			}
			ver := buf[8]
			refID := binary.BigEndian.Uint32(buf[12:16])
			ts := binary.BigEndian.Uint32(buf[16:20])
			if ts == 0 {
				return -1, fmt.Errorf("fmp4: sidx track %d has zero timescale", refID)
			}
			tsByTrack[refID] = ts
			delta := scaleDelta(deltaUs, ts)
			if ver == 0 {
				ept := uint64(binary.BigEndian.Uint32(buf[20:24]))
				nv := ept + uint64(delta)
				if nv > 0xFFFFFFFF {
					return -1, fmt.Errorf("fmp4: sidx v0 EPT overflow (track %d, %d + %d)", refID, ept, delta)
				}
				patches = append(patches, patch{off + 20, 4, nv})
			} else {
				ept := binary.BigEndian.Uint64(buf[20:28])
				patches = append(patches, patch{off + 20, 8, ept + uint64(delta)})
			}
		case "moof":
			buf := make([]byte, size)
			if _, err := f.ReadAt(buf, off); err != nil {
				return -1, err
			}
			var walkErr error
			childBoxes(buf, 8, int(size), func(t string, to, tsz int) bool {
				if t != "traf" {
					return true
				}
				var trackID uint32
				tfdtOff, tfdtVer, tfdtVal := -1, byte(0), uint64(0)
				childBoxes(buf, to+8, to+tsz, func(t2 string, co, csz int) bool {
					switch t2 {
					case "tfhd":
						if csz >= 16 {
							trackID = binary.BigEndian.Uint32(buf[co+12 : co+16])
						}
					case "tfdt":
						if csz < 12 {
							return true // truncated: cannot read version byte
						}
						tfdtVer = buf[co+8]
						if tfdtVer == 1 && csz >= 20 {
							tfdtVal = binary.BigEndian.Uint64(buf[co+12 : co+20])
							tfdtOff = co + 12
						} else if tfdtVer == 0 && csz >= 16 {
							tfdtVal = uint64(binary.BigEndian.Uint32(buf[co+12 : co+16]))
							tfdtOff = co + 12
						}
					}
					return true
				})
				if tfdtOff < 0 {
					return true // traf without tfdt (spec-legal) - nothing to shift
				}
				ts, ok := tsByTrack[trackID]
				if !ok {
					walkErr = fmt.Errorf("fmp4: no timescale for track %d (no sidx, no init fallback)", trackID)
					return false
				}
				delta := scaleDelta(deltaUs, ts)
				nv := tfdtVal + uint64(delta)
				if tfdtVer == 0 && nv > 0xFFFFFFFF {
					walkErr = fmt.Errorf("fmp4: tfdt v0 overflow (track %d, %d + %d)", trackID, tfdtVal, delta)
					return false
				}
				width := 8
				if tfdtVer == 0 {
					width = 4
				}
				patches = append(patches, patch{off + int64(tfdtOff), width, nv})
				if firstTfdtMs < 0 {
					firstTfdtMs = int64(nv * 1000 / uint64(ts)) //nolint:gosec // bounded: media timestamps
				}
				return true
			})
			if walkErr != nil {
				return -1, walkErr
			}
		}
		off += size
	}

	if deltaUs == 0 {
		// Verification pass (cold copy sessions): timestamps are already
		// absolute; nothing to write, only firstTfdtMs is consumed.
		return firstTfdtMs, nil
	}
	var scratch [8]byte
	for _, p := range patches {
		if p.width == 4 {
			binary.BigEndian.PutUint32(scratch[:4], uint32(p.val))
		} else {
			binary.BigEndian.PutUint64(scratch[:8], p.val)
		}
		if _, err := f.WriteAt(scratch[:p.width], p.off); err != nil {
			return -1, err
		}
	}
	return firstTfdtMs, nil
}

// parseInitTimescales extracts track_ID → mdhd timescale from an fMP4 init
// segment (moov/trak/{tkhd,mdia/mdhd}). Fallback source when a media segment
// carries no sidx.
func parseInitTimescales(path string) (map[uint32]uint32, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := make(map[uint32]uint32, 2)
	childBoxes(buf, 0, len(buf), func(t string, o, s int) bool {
		if t != "moov" {
			return true
		}
		childBoxes(buf, o+8, o+s, func(t2 string, o2, s2 int) bool {
			if t2 != "trak" {
				return true
			}
			var trackID uint32
			var timescale uint32
			childBoxes(buf, o2+8, o2+s2, func(t3 string, o3, s3 int) bool {
				switch t3 {
				case "tkhd":
					idOff := o3 + 20 // v0: 12 hdr + creation(4)+modification(4)
					if buf[o3+8] == 1 {
						idOff = o3 + 28 // v1: 12 hdr + creation(8)+modification(8)
					}
					if idOff+4 <= o3+s3 {
						trackID = binary.BigEndian.Uint32(buf[idOff : idOff+4])
					}
				case "mdia":
					childBoxes(buf, o3+8, o3+s3, func(t4 string, o4, s4 int) bool {
						if t4 != "mdhd" {
							return true
						}
						tsOff := o4 + 20
						if buf[o4+8] == 1 {
							tsOff = o4 + 28
						}
						if tsOff+4 <= o4+s4 {
							timescale = binary.BigEndian.Uint32(buf[tsOff : tsOff+4])
						}
						return false
					})
				}
				return true
			})
			if trackID != 0 && timescale != 0 {
				out[trackID] = timescale
			}
			return true
		})
		return false
	})
	if len(out) == 0 {
		return nil, errors.New("fmp4: no track timescales found in init")
	}
	return out, nil
}

// fmp4CompleteMoov reports whether path holds a fully-written moov box -
// hlsenc writes the init segment directly (no temp_file rename), so a scan
// can observe it mid-write. A moov whose declared size fits the file proves
// the write completed (hlsenc writes ftyp+moov sequentially then closes).
func fmp4CompleteMoov(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || fi.Size() < 16 {
		return false
	}
	var off int64
	for off < fi.Size() {
		typ, size, err := boxHeader(f, off, fi.Size())
		if err != nil {
			return false
		}
		if typ == "moov" {
			return true // boxHeader already proved off+size <= fileSize
		}
		off += size
	}
	return false
}

// copyDriftToleranceMs bounds the acceptable gap between a copy segment's
// produced timeline (first traf tfdt) and the keyframe plan's start for that
// index. Generous on purpose: legitimate skews are tiny (ms-rounded keyframe
// times, the +50 ms seek nudge, AAC priming ~21 ms, audio-first layouts
// ~60 ms), while every historical regression of this contract was >= several
// hundred ms (a GOP or more). 500 ms separates the two cleanly.
const copyDriftToleranceMs = 500

// timelineDriftReporter is one rebase pump's divergence log budget. A broken
// plan<->ffmpeg contract diverges on EVERY segment, so the tripwire is a mass
// event by nature: the leading-keyframe plan bug alone produced 1238 of the
// 1239 ERRORs of a 15-day prod window, burying the single real one. The metric
// carries the volume (incremented on every divergence); the log carries the
// news - the FIRST divergence of a session at ERROR, the rest at Debug, plus
// one Warn summary with the count when the pump exits.
//
// Not concurrency-safe by design: only the pump goroutine's sweep calls it.
type timelineDriftReporter struct {
	seen int
	kind string
}

// verify is the runtime tripwire for the rebase pump's plan<->ffmpeg contract:
// the published segment stream_<v>_<idx>.m4s must carry a timeline starting at
// the expected position for <idx>. For the COPY rung that oracle is the
// keyframe plan; for a TRANSCODE-SEEK session it is the linear idx×segment-
// length position. Every desync bug of 2026-06-10 (session-relative split grid,
// off-keyframe seek landings, trimmed-but-mislabeled audio) would have tripped
// this. The segment is still published (a labeled, observable degradation beats
// a stalled player).
// No ctx parameter: called from runRebasePump, which only has done/stop
// channels (shared helper, signature change out of scope for this pass).
func (r *timelineDriftReporter) verify(name, segPrefix string, tfdtMs int64, expectedStartMs func(int) (int64, bool), driftKind string) {
	if expectedStartMs == nil || tfdtMs < 0 {
		return
	}
	idxStr, ok := strings.CutPrefix(name, segPrefix)
	if !ok {
		return
	}
	idxStr, ok = strings.CutSuffix(idxStr, ".m4s")
	if !ok {
		return
	}
	idx, err := strconv.Atoi(idxStr)
	if err != nil {
		return
	}
	want, known := expectedStartMs(idx)
	if !known {
		return
	}
	diff := tfdtMs - want
	if diff <= copyDriftToleranceMs && diff >= -copyDriftToleranceMs {
		return
	}

	r.seen++
	r.kind = driftKind
	msg := "hls: copy segment timeline DIVERGES from the keyframe plan (plan<->ffmpeg contract regression)"
	wantKey := "plan_start_ms"
	if driftKind == "transcode_seek" {
		metrics.HLSVODTranscodeSeekTimelineDrift.Inc()
		msg = "hls: transcode-seek segment timeline DIVERGES from the expected linear position (rebase contract regression)"
		wantKey = "expected_start_ms"
	} else {
		metrics.HLSVODCopyTimelineDrift.Inc()
	}
	attrs := []any{
		slog.String("segment", name),
		slog.Int64("produced_start_ms", tfdtMs),
		slog.Int64(wantKey, want),
		slog.Int64("drift_ms", diff),
	}
	if r.seen > 1 {
		// Same session, same contract break: keep the metric, drop the noise.
		slog.Debug(msg, append(attrs, slog.Int("repeat", r.seen))...)
		return
	}
	slog.Error(msg, append(attrs, slog.Bool("further_repeats_demoted_to_debug", true))...)
}

// summarize reports the session's divergence total once, at pump exit, so the
// volume stays visible in the logs without one line per segment.
func (r *timelineDriftReporter) summarize(privDir string) {
	if r.seen < 2 {
		return
	}
	slog.Warn("hls: segment timeline diverged for the whole session",
		slog.String("dir", privDir),
		slog.String("drift_kind", r.kind),
		slog.Int("diverged_segments", r.seen))
}

// waitProcExit blocks until the session's ffmpeg has really exited, or the
// grace period lapses. A nil channel (call sites that do not track the process)
// returns immediately, preserving the previous behaviour for them.
func waitProcExit(procExited <-chan struct{}) {
	if procExited == nil {
		return
	}
	t := time.NewTimer(procExitGrace)
	defer t.Stop()
	select {
	case <-procExited:
	case <-t.C:
		slog.Warn("hls: rebase pump gave up waiting for ffmpeg to exit; removing its scratch dir anyway")
	}
}

// runRebasePump watches privDir for segments ffmpeg completed (temp_file
// rename done → final name present without a `.tmp` sibling), rebases their
// fMP4 timestamps to the session's absolute start, and atomically renames
// them into pubDir, which is the SegmentCache directory readers poll. The
// init segment is published only if no canonical init exists yet (first
// session wins; per-session inits differ only in edit lists hls.js ignores).
// Exits on ctx cancel or ffmpeg termination, after a final sweep, and removes
// privDir.
// namePrefix selects the rung's filenames: "stream" for the video variants,
// "audio" for the EXT-X-MEDIA renditions (which seek exactly the same way).
// procExitGrace bounds the wait for a cancelled ffmpeg to actually die before
// its scratch directory is removed. The kill is a SIGKILL from
// exec.CommandContext, so this is microseconds in practice; the bound only
// exists so a wedged process cannot leak the directory forever.
const procExitGrace = 5 * time.Second

// finishAfterPump runs a seeked session's rebase pump and returns the channel
// its handle's Done() reports: closed once the process is over (done) AND the
// pump has published what it left. Done() is what the orchestrators reap on;
// fired with the process exit, it raced the pump's last sweep and the final
// segment of a session run to the end of the file was answered 503 while it
// was being published.
func finishAfterPump(done <-chan struct{}, pump func()) chan struct{} {
	pumped := make(chan struct{})
	go func() {
		defer close(pumped)
		pump()
	}()
	finished := make(chan struct{})
	go func() {
		<-done
		<-pumped
		close(finished)
	}()
	return finished
}

func runRebasePump(done <-chan struct{}, stop <-chan struct{}, procExited <-chan struct{}, privDir, pubDir string, deltaUs int64, namePrefix string, variantIdx int, expectedStartMs func(int) (int64, bool), driftKind string) {
	// ADR-0007: both call sites spawn this as a bare goroutine, and a parent
	// recover never covers a child goroutine - a panic here (file I/O races,
	// box parsing on a truncated segment) would kill the whole server.
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic recovered in fmp4 rebase pump",
				slog.String("dir", privDir),
				slog.Any("recover", r), slog.String("stack", string(debug.Stack())))
		}
	}()
	initName := fmt.Sprintf("%s_%d_init.mp4", namePrefix, variantIdx)
	segPrefix := fmt.Sprintf("%s_%d_", namePrefix, variantIdx)
	published := make(map[string]bool, 64)
	failures := make(map[string]int, 4)
	var initTS map[uint32]uint32
	drift := &timelineDriftReporter{}
	defer drift.summarize(privDir)

	sweep := func() {
		entries, err := os.ReadDir(privDir)
		if err != nil {
			return
		}
		for _, e := range entries {
			name := e.Name()
			if published[name] || e.IsDir() ||
				strings.HasSuffix(name, ".tmp") ||
				strings.HasSuffix(name, ".m3u8") ||
				strings.HasPrefix(name, ".") {
				continue
			}
			src := filepath.Join(privDir, name)
			// A multi-variant session writes one init PER rung into the same dir;
			// every one of them must reach the cache, not only variantIdx's.
			if strings.HasPrefix(name, namePrefix+"_") && strings.HasSuffix(name, "_init.mp4") {
				if !fmp4CompleteMoov(src) {
					continue // still being written
				}
				if initTS == nil && name == initName {
					if ts, err := parseInitTimescales(src); err == nil {
						initTS = ts // best-effort fallback map
					}
				}
				dst := filepath.Join(pubDir, name)
				if fmp4CompleteMoov(dst) {
					_ = os.Remove(src) //nolint:errcheck // canonical init already pinned; best-effort cleanup
				} else if err := os.Rename(src, dst); err != nil {
					continue
				}
				published[name] = true
				continue
			}
			if !strings.HasSuffix(name, ".m4s") {
				continue
			}
			if _, err := os.Stat(src + ".tmp"); err == nil {
				continue // defence in depth; rename normally beats the scan
			}
			tfdtMs, err := rebaseFMP4Segment(src, deltaUs, initTS)
			if err != nil {
				failures[name]++
				if failures[name] == 3 {
					// runRebasePump takes done/stop channels, not a ctx (shared by two
					// call sites + a test); threading a ctx through is a signature change.
					slog.Error("hls: copy rebase failed, segment withheld",
						slog.String("segment", src), slog.Any("err", err))
					published[name] = true // stop retrying; Wait times out → re-spawn
				}
				continue
			}
			drift.verify(name, segPrefix, tfdtMs, expectedStartMs, driftKind)
			if err := os.Rename(src, filepath.Join(pubDir, name)); err != nil {
				slog.Warn("hls: copy rebase publish failed",
					slog.String("segment", name), slog.Any("err", err))
				continue
			}
			published[name] = true
		}
	}

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			// Let ffmpeg die BEFORE its scratch directory goes. Removing it
			// first is what produced the burst of `audio-only exited with
			// error / exit status 254` on every seek: the superseded process
			// was still writing segments into the directory the pump had just
			// deleted, so it died on ENOENT and dumped a page of muxer errors
			// instead of exiting on its kill. The sweep after the wait can also
			// still publish a segment the process finished on its way out.
			waitProcExit(procExited)
			sweep()
			_ = os.RemoveAll(privDir)
			return
		case <-done:
			sweep() // ffmpeg just renamed its last segment - catch it
			_ = os.RemoveAll(privDir)
			return
		case <-ticker.C:
			sweep()
		}
	}
}
