package mkvgo

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	extmkv "github.com/gravity-zero/mkvgo/mkv"
	"github.com/gravity-zero/mkvgo/mkv/ops"
	"github.com/gravity-zero/mkvgo/mkv/reader"

	"github.com/evey-project/evey/internal/domain/mkv"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// durToleranceMs is the slack allowed when comparing the rebuilt file's duration
// against the original. Reindex copies cluster payloads verbatim so the duration
// is identical in practice; the tolerance only absorbs rounding in how the two
// reads derive DurationMs.
const durToleranceMs = 50

// HasSeekIndex reports whether the EBML file at path already has a Cues index.
func (a *Adapter) HasSeekIndex(ctx context.Context, path string) (bool, error) {
	ctx, span := tracing.StartSpan(ctx, "mkvgo.HasSeekIndex")
	defer span.End()

	if !IsEBMLContainer(path) {
		return false, nil
	}
	c, err := reader.Open(ctx, path)
	if err != nil {
		return false, fmt.Errorf("mkvgo has-seek-index: %w", err)
	}
	return len(c.Cues) > 0, nil
}

// ReindexInPlace rebuilds path's seek index without ever risking the original.
// See the mkv.Reindexer port for the contract. The ordering is deliberate:
//
//  1. open the source read-only and bail out early if it already has an index;
//  2. Reindex to a sibling temp file (the source is only ever read);
//  3. fully re-parse + Validate the temp (walks every block) and refuse on any
//     error-severity issue;
//  4. cross-check the temp against the source (track count, a freshly-present
//     index, matching duration);
//  5. rename the original to "<path>.mkvgobak", then rename the temp into place;
//     if the second rename fails the backup is renamed back, so the original is
//     always recoverable.
// removeQuiet deletes path best-effort. A missing file is fine (the temp may
// never have been created); any other failure is logged but never fatal — these
// are cleanup paths where the caller already holds its real result.
func removeQuiet(path string) {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Warn("mkvgo: temp cleanup failed", slog.String("path", path), slog.Any("err", err))
	}
}

func (a *Adapter) ReindexInPlace(ctx context.Context, path string, opts mkv.ReindexOptions) (mkv.ReindexResult, error) {
	ctx, span := tracing.StartSpan(ctx, "mkvgo.ReindexInPlace")
	defer span.End()

	if !IsEBMLContainer(path) {
		return mkv.ReindexResult{Skipped: true}, nil
	}

	src, err := reader.Open(ctx, path)
	if err != nil {
		return mkv.ReindexResult{}, fmt.Errorf("reindex: open source: %w", err)
	}
	if len(src.Cues) > 0 {
		return mkv.ReindexResult{Skipped: true}, nil // already indexed — nothing to do
	}
	srcTracks := len(src.Tracks)
	srcDurMs := src.DurationMs

	srcInfo, err := os.Stat(path)
	if err != nil {
		return mkv.ReindexResult{}, fmt.Errorf("reindex: stat source: %w", err)
	}

	// (2) Reindex to a sibling temp. Same directory → same filesystem, so the
	// final swap is a true atomic rename.
	tmp := path + ".reindex.tmp"
	removeQuiet(tmp) // clear any stale temp from a previous aborted run
	if err = ops.Reindex(ctx, path, tmp); err != nil {
		removeQuiet(tmp)
		return mkv.ReindexResult{}, fmt.Errorf("reindex: rebuild: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			removeQuiet(tmp)
		}
	}()

	// (3) Validate the rebuilt file: this re-parses the header/tracks AND walks
	// every cluster block, so a corrupt rebuild surfaces as an error issue.
	issues, err := ops.Validate(ctx, tmp)
	if err != nil {
		return mkv.ReindexResult{}, fmt.Errorf("reindex: validate output: %w", err)
	}
	var warnings []string
	for _, iss := range issues {
		if iss.Severity == extmkv.SeverityError {
			return mkv.ReindexResult{}, fmt.Errorf("reindex: output failed validation: %s", iss.Message)
		}
		warnings = append(warnings, iss.Message)
	}

	// (4) Cross-check the rebuild against the original.
	out, err := reader.Open(ctx, tmp)
	if err != nil {
		return mkv.ReindexResult{}, fmt.Errorf("reindex: re-open output: %w", err)
	}
	if len(out.Tracks) != srcTracks {
		return mkv.ReindexResult{}, fmt.Errorf("reindex: track count changed (%d -> %d), refusing swap", srcTracks, len(out.Tracks))
	}
	if len(out.Cues) == 0 {
		return mkv.ReindexResult{}, fmt.Errorf("reindex: output still has no seek index, refusing swap")
	}
	if srcDurMs > 0 && out.DurationMs > 0 {
		if d := srcDurMs - out.DurationMs; d > durToleranceMs || d < -durToleranceMs {
			return mkv.ReindexResult{}, fmt.Errorf("reindex: duration changed (%dms -> %dms), refusing swap", srcDurMs, out.DurationMs)
		}
	}

	tmpInfo, err := os.Stat(tmp)
	if err != nil {
		return mkv.ReindexResult{}, fmt.Errorf("reindex: stat output: %w", err)
	}

	// (5) Atomic swap with a recoverable backup.
	bak := path + ".mkvgobak"
	if err := os.Rename(path, bak); err != nil {
		return mkv.ReindexResult{}, fmt.Errorf("reindex: back up original: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		if rerr := os.Rename(bak, path); rerr != nil {
			// Both failed: the original is safe at bak but not at path. Surface
			// loudly with the backup location so an operator can restore by hand.
			return mkv.ReindexResult{}, fmt.Errorf("reindex: swap failed AND restore failed, original preserved at %q: swap=%w restore=%w", bak, err, rerr)
		}
		return mkv.ReindexResult{}, fmt.Errorf("reindex: swap failed, original restored: %w", err)
	}
	committed = true

	res := mkv.ReindexResult{
		BackupPath: bak,
		Warnings:   warnings,
		SrcBytes:   srcInfo.Size(),
		DstBytes:   tmpInfo.Size(),
	}
	if !opts.KeepBackup {
		if err := os.Remove(bak); err != nil {
			slog.Warn("mkvgo: reindex backup cleanup failed (left in place)",
				slog.String("backup", bak), slog.Any("err", err))
		} else {
			res.BackupPath = ""
		}
	}
	return res, nil
}
