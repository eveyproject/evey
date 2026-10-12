package mkvgo

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gravity-zero/mkvgo/matroska"
	extmkv "github.com/gravity-zero/mkvgo/mkv"
	"github.com/gravity-zero/mkvgo/mkv/reader"

	"github.com/eveyproject/evey/internal/domain/mkv"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// assCodecs lists the codec identifiers (as returned by mkvgo) that can be
// extracted as ASS/SSA. ExtractSubtitle rejects ASS-format requests for any
// codec not in this set.
var assCodecs = map[string]bool{"ass": true, "ssa": true}

// ExtractSubtitle extracts an embedded subtitle track from a Matroska
// container to a standalone file (SRT or ASS), delegating to the stable
// matroska façade.
func (a *Adapter) ExtractSubtitle(ctx context.Context, req mkv.ExtractSubtitleRequest) error {
	ctx, span := tracing.StartSpan(ctx, "mkvgo.ExtractSubtitle")
	defer span.End()

	if !IsEBMLContainer(req.SrcPath) {
		return fmt.Errorf("mkvgo extract subtitle: %w: %s", mkv.ErrNotEBML, req.SrcPath)
	}

	format := req.Format
	if format == "" {
		format = inferSubtitleFormat(req.OutPath)
	}

	// Validate track existence and codec before delegating - the error messages
	// from the lower-level library don't map to domain sentinel errors, so we
	// probe up-front and translate ourselves.
	codec, err := subtitleTrackCodec(ctx, req.SrcPath, req.TrackID)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(req.OutPath), 0o755); err != nil {
		return fmt.Errorf("mkvgo extract subtitle: mkdir: %w", err)
	}

	switch format {
	case mkv.SubtitleFormatASS:
		if !assCodecs[codec] {
			return fmt.Errorf("mkvgo extract subtitle: %w: track %d has codec %q, need ass/ssa",
				mkv.ErrUnsupportedCodec, req.TrackID, codec)
		}
		return matroska.ExtractASS(ctx, req.SrcPath, req.TrackID, req.OutPath)

	case mkv.SubtitleFormatSRT:
		// SRT extraction is available for all subtitle tracks; the library
		// emits raw text blocks in SRT format regardless of source codec.
		return matroska.ExtractSubtitle(ctx, req.SrcPath, req.TrackID, req.OutPath)

	default:
		return fmt.Errorf("mkvgo extract subtitle: unknown format %q", format)
	}
}

// inferSubtitleFormat returns SubtitleFormatASS for .ass/.ssa extensions and
// SubtitleFormatSRT for everything else (including the empty string).
func inferSubtitleFormat(path string) mkv.SubtitleFormat {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ass", ".ssa":
		return mkv.SubtitleFormatASS
	default:
		return mkv.SubtitleFormatSRT
	}
}

// subtitleTrackCodec probes the container at srcPath and returns the codec
// string for the subtitle track with the given ID, or ErrTrackNotFound when
// the track doesn't exist or is not a subtitle track.
func subtitleTrackCodec(ctx context.Context, srcPath string, trackID uint64) (string, error) {
	// Tracks live in the head: a codec lookup must not walk the clusters.
	c, err := reader.OpenMeta(ctx, srcPath)
	if err != nil {
		return "", fmt.Errorf("mkvgo open for codec probe: %w", err)
	}
	for _, t := range c.Tracks {
		if t.ID == trackID && t.Type == extmkv.SubtitleTrack {
			return t.Codec, nil
		}
	}
	return "", fmt.Errorf("mkvgo extract subtitle: %w: track %d in %s",
		mkv.ErrTrackNotFound, trackID, srcPath)
}
