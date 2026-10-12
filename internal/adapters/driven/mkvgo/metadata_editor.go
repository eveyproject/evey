package mkvgo

import (
	"context"
	"fmt"

	"github.com/gravity-zero/mkvgo/matroska"

	"github.com/eveyproject/evey/internal/domain/mkv"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// EditMetadata writes a new copy of srcPath to dstPath with the metadata
// mutations from req applied. It uses matroska.EditMetadata which copies
// clusters verbatim (fast path) and rebuilds only the Cues index.
func (a *Adapter) EditMetadata(ctx context.Context, srcPath, dstPath string, req mkv.EditMetadataRequest) error {
	ctx, span := tracing.StartSpan(ctx, "mkvgo.EditMetadata")
	defer span.End()

	if !IsEBMLContainer(srcPath) {
		return fmt.Errorf("mkvgo edit metadata: %w: %s", mkv.ErrNotEBML, srcPath)
	}

	return matroska.EditMetadata(ctx, srcPath, dstPath, applyMetadataEdit(req))
}

// EditInPlace rewrites only the metadata region of path in-place.
// It is fast but NOT crash-safe; callers with precious data should use
// EditMetadata instead (write to a temp file, then atomically rename).
func (a *Adapter) EditInPlace(ctx context.Context, path string, req mkv.EditMetadataRequest) error {
	ctx, span := tracing.StartSpan(ctx, "mkvgo.EditInPlace")
	defer span.End()

	if !IsEBMLContainer(path) {
		return fmt.Errorf("mkvgo edit in-place: %w: %s", mkv.ErrNotEBML, path)
	}

	return matroska.EditInPlace(ctx, path, applyMetadataEdit(req))
}

// applyMetadataEdit returns a closure that mutates a matroska.Container
// according to req. The closure is passed directly to the matroska API.
func applyMetadataEdit(req mkv.EditMetadataRequest) func(*matroska.Container) {
	return func(c *matroska.Container) {
		if req.Title != nil {
			c.Info.Title = *req.Title
		}

		if len(req.TrackEdits) == 0 {
			return
		}

		// Build a lookup map for O(1) edit access.
		edits := make(map[uint64]mkv.TrackEdit, len(req.TrackEdits))
		for _, e := range req.TrackEdits {
			edits[e.TrackID] = e
		}

		for i := range c.Tracks {
			e, ok := edits[c.Tracks[i].ID]
			if !ok {
				continue
			}
			if e.Language != nil {
				c.Tracks[i].Language = *e.Language
			}
			if e.Name != nil {
				c.Tracks[i].Name = *e.Name
			}
			if e.IsDefault != nil {
				c.Tracks[i].IsDefault = *e.IsDefault
			}
			if e.IsForced != nil {
				c.Tracks[i].IsForced = *e.IsForced
			}
		}
	}
}
