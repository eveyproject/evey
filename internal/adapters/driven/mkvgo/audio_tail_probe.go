package mkvgo

// audio_tail_probe.go - does the audio content run as far as the VIDEO does?
// The defect this measures (prod 2026-07-26, "Scary Movie"): a mux whose audio
// track dies minutes before the video's end. The file is structurally healthy -
// index fine, sizes coherent - so ops.Diagnose cannot see it, yet the VOD
// playlists (synthesized from the declared duration) promise audio segments
// that can never exist.
//
// The reference is the video's own end, NOT the container's declared duration.
// That duration is a number some muxer wrote, and a bad remux over-states it
// routinely (duration inherited from an uncut source, a botched concat). When
// it is the liar, the video stops early too - and measuring the audio against
// it produced a confident "Audio -154s" on files whose tracks were perfectly
// aligned. Comparing content to content, one track against another, is the only
// claim this probe can actually stand behind. The declared duration is kept
// only as a ceiling, and as the sole reference for an audio-only container.
//
// mkvgo-only, two stages, cheapest first:
//  1. HEAD: mkvmerge and ffmpeg stamp every track with a DURATION tag
//     (TagTrackUID-targeted SimpleTag); when every audio AND video track
//     carries one, a single metadata read settles the gap.
//  2. TAIL: otherwise seek to a Cue point a window before the declared end
//     and walk the final clusters, keeping each track's LAST block timecode -
//     widening the window in steps so a deep gap is still measured without
//     ever scanning the whole file.

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

// ProbeAudioTailGapSec returns the largest gap, in seconds, between where the
// video content ends and where each audio track's content ends. 0 = every
// audio track runs as far as the video, or the question cannot be answered
// cheaply (no declared duration, no audio, or - for the tail walk - no Cue
// index to seek with; such files are already flagged by the index triage).
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
	var audio, video []extmkv.Track
	for _, t := range meta.Tracks {
		switch t.Type {
		case extmkv.AudioTrack:
			audio = append(audio, t)
		case extmkv.VideoTrack:
			video = append(video, t)
		}
	}
	if len(audio) == 0 {
		return 0, nil
	}

	// The video tracks are measured too, and the fast path needs ALL of them
	// tagged: a gap computed from a tagged audio track against an untagged
	// video one would silently fall back on the declared duration, which is the
	// very reference this probe stopped trusting. Muxers tag every track or
	// none, so the fast path is not lost in practice.
	ends := trackTagEnds(meta, audio, video)
	if len(ends) == len(audio)+len(video) {
		return tailGap(durSec, audio, video, ends), nil
	}
	return probeAudioTailGapFromClusters(ctx, path, meta, audio, video, ends)
}

// tailGap turns per-track end times into the reported shortfall: how far the
// worst audio track falls short of the reference end. The reference is where
// the VIDEO stops, capped by the declared duration (a video running past the
// declaration is a different anomaly, not this probe's to report). With no
// video track - an .mka - the declaration is the only reference there is.
func tailGap(durSec float64, audio, video []extmkv.Track, ends map[uint64]float64) float64 {
	ref := durSec
	videoEnd, measured := 0.0, false
	for _, tr := range video {
		if e, ok := ends[tr.ID]; ok {
			measured = true
			if e > videoEnd {
				videoEnd = e
			}
		}
	}
	if measured && videoEnd < ref {
		ref = videoEnd
	}
	gap := 0.0
	for _, tr := range audio {
		end, ok := ends[tr.ID]
		if !ok {
			continue
		}
		if g := ref - end; g > gap {
			gap = g
		}
	}
	return gap
}

// trackTagEnds resolves each track's end time from its DURATION tag
// (TagTrackUID-targeted). Tracks without a parseable tag are absent from the
// result - the caller falls back to the tail walk for those.
func trackTagEnds(meta *extmkv.Container, groups ...[]extmkv.Track) map[uint64]float64 {
	ends := make(map[uint64]float64)
	for _, tr := range concatTracks(groups...) {
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
// window before the declared end and record each track's last block timecode -
// video included, since it is the reference the audio is judged against -
// widening the window until every untagged track has answered. A track still
// silent after the widest window is placed AT the window start: a lower bound,
// and the same one for video and audio, so a file whose picture and sound both
// stop early reports no gap rather than a fabricated one.
func probeAudioTailGapFromClusters(ctx context.Context, path string, meta *extmkv.Container, audio, video []extmkv.Track, tagEnds map[uint64]float64) (float64, error) {
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
	tracks := concatTracks(audio, video)

	pending := make(map[uint64]bool)
	for _, tr := range tracks {
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

	ends := make(map[uint64]float64, len(tracks))
	for _, tr := range tracks {
		tagged, hasTag := tagEnds[tr.ID]
		ticks, walked := lastTicks[tr.ID]
		switch {
		case hasTag:
			ends[tr.ID] = tagged
		case walked:
			ends[tr.ID] = float64(ticks) * float64(scale) / 1e9
		default:
			// No block inside the widest window walked: the track ends at
			// least there. Lower bound, already far past any threshold.
			ends[tr.ID] = float64(windowStartMs) / 1000
		}
	}
	return tailGap(durSec, audio, video, ends), nil
}

// concatTracks flattens the audio/video groups into one slice - the several
// places that must treat "every track we measure" uniformly.
func concatTracks(groups ...[]extmkv.Track) []extmkv.Track {
	var n int
	for _, g := range groups {
		n += len(g)
	}
	out := make([]extmkv.Track, 0, n)
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
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
