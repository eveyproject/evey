package mkvgo

// audio_tail_probe.go - does the audio content actually reach the container's
// declared duration? The defect this measures (prod 2026-07-26, "Scary
// Movie"): a mux whose audio track dies minutes before the video's end. The
// file is structurally healthy - index fine, sizes coherent - so ops.Diagnose
// cannot see it, yet the VOD playlists (synthesized from the DECLARED
// duration) promise audio segments that can never exist.
//
// mkvgo-only, two stages, cheapest first:
//  1. HEAD: mkvmerge and ffmpeg stamp every track with a DURATION tag
//     (TagTrackUID-targeted SimpleTag); when every audio track carries one,
//     one metadata read settles the gap.
//  2. TAIL: otherwise seek to a Cue point a window before the declared end
//     and walk the final clusters, keeping each audio track's LAST block
//     timecode - widening the window in steps so a deep gap is still
//     measured without ever scanning the whole file.

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/gravity-zero/mkvgo/ebml"
	extmkv "github.com/gravity-zero/mkvgo/mkv"
	"github.com/gravity-zero/mkvgo/mkv/reader"
)

// audioTailWindowsSec are the successive "walk the last N seconds" windows of
// the tail fallback. The first covers ordinary files (audio runs to the end);
// the second measures deep gaps (the motivating file was 249 s short). A track
// still silent past the last window is reported AT the window - a lower bound,
// already far past any flagging threshold.
var audioTailWindowsSec = []int64{120, 900}

// audioTailWalkCap bounds the bytes a tail walk may read, a guard against a
// pathological file whose final "window" of clusters is enormous.
const audioTailWalkCap = int64(2) << 30 // 2 GiB

// audioTailMaxClusterSize mirrors the reindex walker's per-cluster sanity cap.
const audioTailMaxClusterSize = int64(512) << 20 // 512 MiB

// AudioTailProber measures how far before the declared duration each audio
// track's content ends. Stateless; satisfies maintenance.AudioTailProber.
type AudioTailProber struct{}

func NewAudioTailProber() *AudioTailProber { return &AudioTailProber{} }

// ProbeAudioTailGapSec returns the largest gap, in seconds, between the
// container's declared duration and the last audio content of any audio
// track. 0 = every audio track reaches the end, or the question cannot be
// answered cheaply (no declared duration, no audio, or - for the tail walk -
// no Cue index to seek with; such files are already flagged by the index
// triage).
func (p *AudioTailProber) ProbeAudioTailGapSec(ctx context.Context, path string) (float64, error) {
	// WithTags: the DURATION fast path reads them. WithCues: the tail walk
	// seeks with them - both are dropped from the metadata path by default.
	meta, err := reader.OpenMeta(ctx, path, reader.WithTags(), reader.WithCues())
	if err != nil {
		return 0, fmt.Errorf("audio tail probe: %w", err)
	}
	durSec := float64(meta.DurationMs) / 1000
	if durSec <= 0 {
		return 0, nil
	}
	var audio []extmkv.Track
	for _, t := range meta.Tracks {
		if t.Type == extmkv.AudioTrack {
			audio = append(audio, t)
		}
	}
	if len(audio) == 0 {
		return 0, nil
	}

	ends := audioTagEnds(meta, audio)
	if len(ends) == len(audio) {
		gap := 0.0
		for _, end := range ends {
			if g := durSec - end; g > gap {
				gap = g
			}
		}
		return gap, nil
	}
	return probeAudioTailGapFromClusters(ctx, path, meta, audio, ends)
}

// audioTagEnds resolves each audio track's end time from its DURATION tag
// (TagTrackUID-targeted). Tracks without a parseable tag are absent from the
// result - the caller falls back to the tail walk for those.
func audioTagEnds(meta *extmkv.Container, audio []extmkv.Track) map[uint64]float64 {
	ends := make(map[uint64]float64)
	for _, tr := range audio {
		if tr.UID == 0 {
			continue
		}
		for _, tag := range meta.Tags {
			if tag.TargetID != tr.UID {
				continue
			}
			for _, st := range tag.SimpleTags {
				if !strings.EqualFold(st.Name, "DURATION") {
					continue
				}
				if sec, ok := durationTagSeconds(st.Value); ok {
					ends[tr.ID] = sec
				}
			}
		}
	}
	return ends
}

// durationTagSeconds parses the Matroska DURATION tag form
// "HH:MM:SS.nnnnnnnnn" into seconds.
func durationTagSeconds(s string) (float64, bool) {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) != 3 {
		return 0, false
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	sec, err3 := strconv.ParseFloat(parts[2], 64)
	if err1 != nil || err2 != nil || err3 != nil || h < 0 || m < 0 || sec < 0 {
		return 0, false
	}
	return float64(h)*3600 + float64(m)*60 + sec, true
}

// probeAudioTailGapFromClusters is the tail fallback: seek to a Cue point a
// window before the declared end and record each audio track's last block
// timecode, widening the window until every untagged track has answered. A
// track still silent after the widest window is reported at the window's
// start - a lower bound on its gap.
func probeAudioTailGapFromClusters(ctx context.Context, path string, meta *extmkv.Container, audio []extmkv.Track, tagEnds map[uint64]float64) (float64, error) {
	if len(meta.Cues) == 0 {
		// Nothing to seek with. Such a file is already flagged by the index
		// triage (no-index -> reindex); a head-to-tail walk here could mean
		// reading a whole 40 GB remux per scanned file, so decline instead.
		return 0, nil
	}
	scale := meta.Info.TimecodeScale
	if scale <= 0 {
		scale = 1_000_000
	}
	durSec := float64(meta.DurationMs) / 1000

	pending := make(map[uint64]bool)
	for _, tr := range audio {
		if _, ok := tagEnds[tr.ID]; !ok {
			pending[tr.ID] = true
		}
	}
	lastTicks := make(map[uint64]int64)
	windowStartMs := int64(0)
	for _, w := range audioTailWindowsSec {
		windowStartMs = max(meta.DurationMs-w*1000, 0)
		offset := cueOffsetAtOrBefore(meta, windowStartMs)
		if err := walkTailClusters(ctx, path, offset, pending, lastTicks); err != nil {
			return 0, err
		}
		for id := range pending {
			if _, ok := lastTicks[id]; ok {
				delete(pending, id)
			}
		}
		if len(pending) == 0 || windowStartMs == 0 {
			break
		}
	}

	gap := 0.0
	consider := func(endSec float64) {
		if g := durSec - endSec; g > gap {
			gap = g
		}
	}
	for _, end := range tagEnds {
		consider(end)
	}
	for _, tr := range audio {
		if _, tagged := tagEnds[tr.ID]; tagged {
			continue
		}
		if ticks, ok := lastTicks[tr.ID]; ok {
			consider(float64(ticks) * float64(scale) / 1e9)
		} else {
			// No block inside the widest window walked: the track ends at
			// least there. Lower bound, already far past any threshold.
			consider(float64(windowStartMs) / 1000)
		}
	}
	return gap, nil
}

// cueOffsetAtOrBefore returns the absolute file offset of the last Cue point
// at or before wantMs (the first Cue when none is). Cue ClusterPos is
// relative to the Segment body.
func cueOffsetAtOrBefore(meta *extmkv.Container, wantMs int64) int64 {
	best := meta.Cues[0].ClusterPos
	for _, c := range meta.Cues {
		if c.TimeMs <= wantMs && c.ClusterPos > best {
			best = c.ClusterPos
		}
	}
	return meta.SegmentStart + best
}

// walkTailClusters reads clusters from offset to EOF, recording the LAST block
// timecode (ticks) of every track in want. Tolerant: a truncated or damaged
// tail stops the walk and keeps what was measured (this runs on defective
// files by design).
func walkTailClusters(ctx context.Context, path string, offset int64, want map[uint64]bool, lastTicks map[uint64]int64) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("audio tail probe: open: %w", err)
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return fmt.Errorf("audio tail probe: seek: %w", err)
	}
	r := bufio.NewReaderSize(f, 1<<20)

	var buf []byte
	var walked int64
	for walked < audioTailWalkCap {
		if err := ctx.Err(); err != nil {
			return err
		}
		h, _, err := ebml.ReadElementHeader(r)
		if err != nil {
			return nil // EOF or an unreadable tail: keep what was measured
		}
		if h.Size < 0 || h.Size > audioTailMaxClusterSize {
			return nil
		}
		if h.ID != extmkv.IDCluster {
			// Top-level non-cluster elements (Cues, Tags, Attachments live at
			// the tail of many files) - skip and keep walking.
			if _, err := io.CopyN(io.Discard, r, h.Size); err != nil {
				return nil
			}
			continue
		}
		if int(h.Size) > len(buf) {
			buf = make([]byte, h.Size)
		}
		body := buf[:h.Size]
		n, rerr := io.ReadFull(r, body)
		// A cut-off cluster still carries blocks up to the cut: scan the
		// prefix that was read, then stop.
		scanClusterLastTimes(body[:n], want, lastTicks)
		if rerr != nil {
			return nil
		}
		walked += h.Size
	}
	return nil
}

// scanClusterLastTimes updates lastTicks from one cluster body, keeping the
// LATEST timecode per wanted track - the mirror of the audio-delay probe's
// first-times scan.
func scanClusterLastTimes(body []byte, want map[uint64]bool, lastTicks map[uint64]int64) {
	r := newSliceReader(body)
	var clusterTS int64
	record := func(track uint64, relTC int16) {
		if !want[track] {
			return
		}
		abs := clusterTS + int64(relTC)
		if cur, ok := lastTicks[track]; !ok || abs > cur {
			lastTicks[track] = abs
		}
	}
	for {
		h, _, err := ebml.ReadElementHeader(r)
		if err != nil || h.Size < 0 || h.Size > int64(r.Len()) {
			return
		}
		switch h.ID {
		case extmkv.IDTimestamp:
			v, err := ebml.ReadUint(r, h.Size)
			if err != nil {
				return
			}
			clusterTS = int64(v)
		case extmkv.IDSimpleBlock:
			track, relTC, ok := readTailBlockHeader(r, h.Size)
			if !ok {
				return
			}
			record(track, relTC)
		case extmkv.IDBlockGroup:
			if !scanTailBlockGroup(r, h.Size, record) {
				return
			}
		default:
			if !r.skip(h.Size) {
				return
			}
		}
	}
}

// scanTailBlockGroup descends one BlockGroup and forwards its Block's
// (track, relTC) to record. Returns false when the body is unreadable.
func scanTailBlockGroup(r *sliceReader, size int64, record func(uint64, int16)) bool {
	end := r.consumed() + size
	for r.consumed() < end {
		h, _, err := ebml.ReadElementHeader(r)
		if err != nil || h.Size < 0 || r.consumed()+h.Size > end {
			return false
		}
		if h.ID == extmkv.IDBlock {
			track, relTC, ok := readTailBlockHeader(r, h.Size)
			if !ok {
				return false
			}
			record(track, relTC)
			continue
		}
		if !r.skip(h.Size) {
			return false
		}
	}
	return true
}

// readTailBlockHeader parses a (Simple)Block's leading track VINT + 2-byte
// relative timecode, then skips the rest of the block. Only the header
// matters here - the payload is never read into anything. The track number
// is a marker-removed VINT, the same encoding as an EBML data size (mirrors
// ops' readBlockHeader).
func readTailBlockHeader(r *sliceReader, blockSize int64) (track uint64, relTC int16, ok bool) {
	before := r.consumed()
	trackRaw, _, err := ebml.ReadDataSize(r)
	if err != nil || trackRaw < 0 {
		return 0, 0, false
	}
	var tc [2]byte
	if _, err := io.ReadFull(r, tc[:]); err != nil {
		return 0, 0, false
	}
	rel := int16(uint16(tc[0])<<8 | uint16(tc[1]))
	if rest := blockSize - (r.consumed() - before); rest > 0 {
		if !r.skip(rest) {
			return 0, 0, false
		}
	}
	return uint64(trackRaw), rel, true
}

// sliceReader is a bytes reader that tracks how much has been consumed - the
// BlockGroup walk needs element-relative bookkeeping ReadElementHeader does
// not provide.
type sliceReader struct {
	b   []byte
	pos int64
}

func newSliceReader(b []byte) *sliceReader { return &sliceReader{b: b} }

func (s *sliceReader) Read(p []byte) (int, error) {
	if s.pos >= int64(len(s.b)) {
		return 0, io.EOF
	}
	n := copy(p, s.b[s.pos:])
	s.pos += int64(n)
	return n, nil
}

func (s *sliceReader) ReadByte() (byte, error) {
	if s.pos >= int64(len(s.b)) {
		return 0, io.EOF
	}
	c := s.b[s.pos]
	s.pos++
	return c, nil
}

func (s *sliceReader) Len() int        { return int(int64(len(s.b)) - s.pos) }
func (s *sliceReader) consumed() int64 { return s.pos }
func (s *sliceReader) skip(n int64) bool {
	if n < 0 || s.pos+n > int64(len(s.b)) {
		return false
	}
	s.pos += n
	return true
}
