package mkvgo

import (
	"context"

	"github.com/gravity-zero/mkvgo/matroska"

	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// frame_extractor.go - the mkvgo keyframe fast path for thumbnails/snapshots.
// matroska.ExtractKeyframeSample seeks the video keyframe nearest a timestamp
// through the Cues (a few bounded reads) and returns it decoder-ready (Annex-B
// with parameter sets prepended for H.264/HEVC, or an IVF wrapper for VP8/VP9/
// AV1). mkvgo never decodes - one ffmpeg call turns the packet into an image,
// instead of ffmpeg seeking + decoding the whole GOP up to the position.

// ExtractKeyframePacket returns the encoded keyframe nearest atMs, its codec
// extension, and its real presentation time. Matroska/WebM only; other
// containers (and a Cues-less file) error, which the caller treats as the
// signal to fall back to the ffmpeg -ss extractor.
func (a *Adapter) ExtractKeyframePacket(ctx context.Context, filePath string, atMs int64) ([]byte, string, int64, error) {
	ctx, span := tracing.StartSpan(ctx, "mkvgo.ExtractKeyframePacket")
	defer span.End()

	ks, err := matroska.ExtractKeyframeSample(ctx, filePath, atMs)
	if err != nil {
		span.RecordError(err)
		return nil, "", 0, err
	}
	return ks.Data, ks.Ext, ks.PtsMs, nil
}
