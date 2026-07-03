package httpapi

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
)

// frame_extractor_fastpath.go — a FrameExtractor that tries the mkvgo keyframe
// fast path before the ffmpeg -ss extractor. mkvgo seeks the nearest keyframe
// through the container's Cues (bounded reads, no full-file scan) and ffmpeg
// decodes just that one frame — far less work on a weak NAS than seeking and
// decoding the whole GOP up to the exact position.
//
// The fast path returns the KEYFRAME, not the exact requested frame, so it is
// only taken when that keyframe lands within maxSnapshotDriftMs of the request
// (a scrubbing preview tolerates a small drift; anything further falls back to
// the exact ffmpeg decode). Matroska/WebM only — the seek-index extractor is
// EBML; every other container, a too-distant keyframe, or any error falls
// straight through to ExtractFrame, so behaviour is never worse than today.

// maxSnapshotDriftMs bounds how far the fast-path keyframe may sit from the
// requested position before we fall back to an exact decode. ~2 s keeps a
// hover/scrub preview visually on-target.
const maxSnapshotDriftMs = 2000

// keyframeFastPathExts are the containers matroska.ExtractKeyframeSample reads.
var keyframeFastPathExts = map[string]bool{".mkv": true, ".webm": true}

type fastFrameExtractor struct {
	kf       KeyframeExtractor
	dec      FramePacketDecoder
	fallback FrameExtractor
}

// NewFastFrameExtractor composes the mkvgo keyframe extractor and the ffmpeg
// packet decoder over an exact-decode fallback. If kf or dec is nil it degrades
// to the fallback, so wiring is always safe.
func NewFastFrameExtractor(kf KeyframeExtractor, dec FramePacketDecoder, fallback FrameExtractor) FrameExtractor {
	return &fastFrameExtractor{kf: kf, dec: dec, fallback: fallback}
}

func (e *fastFrameExtractor) ExtractFrame(ctx context.Context, filePath string, positionSec float64, width int) ([]byte, error) {
	if e.kf != nil && e.dec != nil && keyframeFastPathExts[strings.ToLower(filepath.Ext(filePath))] {
		atMs := int64(positionSec * 1000)
		packet, ext, ptsMs, err := e.kf.ExtractKeyframePacket(ctx, filePath, atMs)
		switch {
		case err != nil:
			// Common on exotic files (no Cues, odd lacing) — the exact decoder
			// covers it, but keep a trace so an mkvgo regression is visible.
			slog.Debug("snapshot fast path: keyframe extract failed, using exact decoder",
				slog.String("path", filePath), slog.Any("err", err))
		case len(packet) > 0 && abs64(atMs-ptsMs) <= maxSnapshotDriftMs:
			jpeg, derr := e.dec.DecodePacketToJPEG(ctx, packet, ext, width)
			if derr == nil && len(jpeg) > 0 {
				return jpeg, nil
			}
			// Unexpected: mkvgo handed us a keyframe packet ffmpeg can't decode.
			slog.Warn("snapshot fast path: keyframe packet decode failed, using exact decoder",
				slog.String("path", filePath), slog.Any("err", derr))
		}
	}
	return e.fallback.ExtractFrame(ctx, filePath, positionSec, width)
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
