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

// backupSuffix is the extension appended to an original file before a reindex
// swaps the rebuilt version in. Kept in one place so HasBackup/RestoreBackup and
// ReindexInPlace agree.
const backupSuffix = ".mkvgobak"

// HasBackup reports whether path has a reindex backup it can be rolled back to.
func (a *Adapter) HasBackup(_ context.Context, path string) (bool, error) {
	_, err := os.Stat(path + backupSuffix)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// DeleteBackup removes path's reindex backup, reclaiming the duplicated disk
// space once the operator has confirmed the repaired file plays fine.
func (a *Adapter) DeleteBackup(ctx context.Context, path string) error {
	_, span := tracing.StartSpan(ctx, "mkvgo.DeleteBackup")
	defer span.End()

	bak := path + backupSuffix
	if err := os.Remove(bak); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("delete backup: %w at %q", mkv.ErrNoBackup, bak)
		}
		return fmt.Errorf("delete backup: %w", err)
	}
	return nil
}

// errInPlaceUnavailable is kept for the ModeAuto fall-through routing below:
// since the v0.17.0 bump the surgical strategy is live, so this sentinel only
// fires if a future build ever re-stubs reindexSurgical.
var errInPlaceUnavailable = errors.New("in-place reindex not available at this mkvgo pin")

// reindexSurgical patches path's seek index in place (mkvgo v0.17.0+):
// appends a fresh Cues element, repoints the head SeekHead, voids the stale
// index - cluster bytes never move, no on-disk duplicate. mkvgo runs its own
// crash-recovery (journal rollback) before patching, so no extra
// RecoverInPlace call is needed here.
func reindexSurgical(ctx context.Context, path string) error {
	return ops.ReindexInPlace(ctx, path)
}

// surgicalAvailable: the pin carries ops.ReindexInPlace. The admin UI reads
// it (via SupportsInPlace) to offer the in-place mode only when it can
// actually run.
const surgicalAvailable = true

// SupportsInPlace reports whether this build carries the surgical strategy.
func (a *Adapter) SupportsInPlace() bool { return surgicalAvailable }

// RestoreBackup atomically replaces path with its backup, undoing a reindex.
func (a *Adapter) RestoreBackup(ctx context.Context, path string) error {
	_, span := tracing.StartSpan(ctx, "mkvgo.RestoreBackup")
	defer span.End()

	bak := path + backupSuffix
	if _, err := os.Stat(bak); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("restore: %w at %q", mkv.ErrNoBackup, bak)
		}
		return fmt.Errorf("restore: stat backup: %w", err)
	}
	// os.Rename is atomic on the same filesystem (the backup sits beside path),
	// so the original is swapped back in a single step and the reindexed file is
	// discarded.
	if err := os.Rename(bak, path); err != nil {
		return fmt.Errorf("restore: swap backup in: %w", err)
	}
	return nil
}

// HasSeekIndex reports whether the EBML file at path already has a Cues index.
// It first tries the head-SeekHead probe (a few KB of reads — what makes a
// whole-library scan feasible); only undecidable files pay the full parse.
func (a *Adapter) HasSeekIndex(ctx context.Context, path string) (bool, error) {
	ctx, span := tracing.StartSpan(ctx, "mkvgo.HasSeekIndex")
	defer span.End()

	if !IsEBMLContainer(path) {
		return false, nil
	}
	if has, confirmed := probeCuesPresence(path); confirmed && !has {
		return false, nil
	}
	// A probe positive only proves a Cues ELEMENT sits at the SeekHead offset.
	// Repacks in the wild ship an EMPTY/garbage Cues body (phantom index):
	// ffmpeg then scans the whole file on any -ss and native players report the
	// stream unseekable - exactly the files a repair must flag. Confirm through
	// OpenMeta, which parses the element's REAL content without the cluster
	// walk (the full reader.Open synthesizes cue points from the walk, so its
	// Cues are non-empty even when the file's index is unusable).
	c, err := reader.OpenMeta(ctx, path, reader.WithCues())
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

	// (1) Bail only when the FILE carries a parseable index. OpenMeta reads the
	// actual Cues element; the full reader below synthesizes cue points from
	// the cluster walk, so ITS Cues are non-empty even for a phantom/empty
	// on-disk index - trusting it skipped exactly the files needing repair.
	meta, err := reader.OpenMeta(ctx, path, reader.WithCues())
	if err != nil {
		return mkv.ReindexResult{}, fmt.Errorf("reindex: open source meta: %w", err)
	}
	if len(meta.Cues) > 0 {
		return mkv.ReindexResult{Skipped: true}, nil // already indexed — nothing to do
	}

	// (2) Surgical in-place first (Mode auto/inplace): needs write access to
	// the FILE only (no temp in the directory), never duplicates it on disk,
	// leaves no backup. In ModeAuto it falls back to the full rewrite below
	// when the strategy is unavailable (pre-bump stub) or refuses this file
	// (streamed clusters, truncated tail, no SeekHead slot) — the in-place
	// journal guarantees the original is intact after any refusal, so the
	// rewrite can safely proceed. In ModeInPlace the operator explicitly
	// refused a rewrite: refusal is surfaced as ErrInPlaceUnsupported.
	// ModeRewrite never attempts the patch.
	if srcInfo, statErr := os.Stat(path); statErr != nil && opts.Mode == mkv.ModeInPlace {
		return mkv.ReindexResult{}, fmt.Errorf("reindex: stat source: %w", statErr)
	} else if statErr == nil && opts.Mode != mkv.ModeRewrite {
		switch inErr := reindexSurgical(ctx, path); {
		case inErr == nil:
			// Tripwire mirroring the rewrite path's step (4): the patched file
			// must now carry a parseable, non-empty Cues element.
			outMeta, vErr := reader.OpenMeta(ctx, path, reader.WithCues())
			if vErr != nil {
				return mkv.ReindexResult{}, fmt.Errorf("reindex: in-place patched file unreadable: %w", vErr)
			}
			if len(outMeta.Cues) == 0 {
				return mkv.ReindexResult{}, fmt.Errorf("reindex: in-place patch left no seek index (contract regression)")
			}
			dstBytes := srcInfo.Size()
			if dstInfo, dErr := os.Stat(path); dErr == nil {
				dstBytes = dstInfo.Size()
			}
			return mkv.ReindexResult{
				Method:   mkv.ReindexMethodInPlace,
				SrcBytes: srcInfo.Size(),
				DstBytes: dstBytes,
			}, nil
		case opts.Mode == mkv.ModeInPlace:
			// The operator demanded the patch: no silent rewrite.
			return mkv.ReindexResult{}, fmt.Errorf("reindex: %w: %w", mkv.ErrInPlaceUnsupported, inErr)
		case errors.Is(inErr, errInPlaceUnavailable):
			// Re-stubbed build (defensive): the rewrite below is the only strategy.
		case errors.Is(inErr, ops.ErrIndexNotHeadDiscoverable):
			// This file's layout defeats the head-only patch (no SeekHead slot,
			// streamed clusters, ...). The journal guarantees the original is
			// untouched, so fall through to the full rewrite - it keeps Evey's
			// backup semantics (.mkvgobak + Validate) unlike ops.ReindexReplace.
			slog.Info("mkvgo: in-place reindex not applicable to this layout, using the full rewrite",
				slog.String("path", path))
		default:
			slog.Info("mkvgo: in-place reindex refused, falling back to the full rewrite",
				slog.String("path", path), slog.Any("err", inErr))
		}
	}

	src, err := reader.Open(ctx, path)
	if err != nil {
		return mkv.ReindexResult{}, fmt.Errorf("reindex: open source: %w", err)
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
	// Same OpenMeta rule as the bail-out: out.Cues (full reader) is synthesized
	// from the cluster walk, so it can't prove the rebuilt FILE carries an
	// index. Read the element's real content.
	outMeta, err := reader.OpenMeta(ctx, tmp, reader.WithCues())
	if err != nil {
		return mkv.ReindexResult{}, fmt.Errorf("reindex: re-open output meta: %w", err)
	}
	if len(outMeta.Cues) == 0 {
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
	bak := path + backupSuffix
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
		Method:     mkv.ReindexMethodRewrite,
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
