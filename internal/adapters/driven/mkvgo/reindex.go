package mkvgo

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"

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

// mkvgoReplaceBackupSuffix is what mkvgo's KeepBackup actually produces; the
// adapter renames it to backupSuffix so the existing rollback/purge/scan-badge
// lifecycle sees it.
const mkvgoReplaceBackupSuffix = ".bak"

// HasBackup reports whether path has a reindex backup it can be rolled back to.
func (a *Adapter) HasBackup(_ context.Context, path string) (bool, error) {
	_, err := os.Stat(path + backupSuffix) //nolint:gosec // operator media path
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
	if err := os.Remove(bak); err != nil { //nolint:gosec // operator media path
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
func reindexSurgical(ctx context.Context, path string, deepVerify bool) error {
	return ops.ReindexInPlace(ctx, path, extmkv.Options{DeepVerify: deepVerify})
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
	if _, err := os.Stat(bak); err != nil { //nolint:gosec // operator media path
		if os.IsNotExist(err) {
			return fmt.Errorf("restore: %w at %q", mkv.ErrNoBackup, bak)
		}
		return fmt.Errorf("restore: stat backup: %w", err)
	}
	// os.Rename is atomic on the same filesystem (the backup sits beside path),
	// so the original is swapped back in a single step and the reindexed file is
	// discarded.
	if err := os.Rename(bak, path); err != nil { //nolint:gosec // operator media path
		return fmt.Errorf("restore: swap backup in: %w", err)
	}
	return nil
}

// HasSeekIndex reports whether the EBML file at path already has a Cues index
// that can serve a video seek - a head-only read, what makes a whole-library
// scan feasible.
func (a *Adapter) HasSeekIndex(ctx context.Context, path string) (bool, error) {
	ctx, span := tracing.StartSpan(ctx, "mkvgo.HasSeekIndex")
	defer span.End()

	if !IsEBMLContainer(path) {
		return false, nil
	}
	// Presence is not usability. Repacks in the wild ship an EMPTY/garbage
	// Cues body (phantom index), one keyed onto the audio track only (every
	// seek then lands mid-GOP), or one so coarse a seek lands a minute from
	// its target - exactly the files a repair must flag. Take CueHealth's
	// WHOLE verdict, not one of its components: judging `VideoCues > 0`
	// answers "is any video cue present?", which "index-sparse" satisfies by
	// definition (see ReindexInPlace). CueHealth reads head-only and finds a
	// Cues element the SeekHead never names by itself (tail scan), so the
	// hand-rolled SeekHead probe that used to short-circuit it is gone.
	ch, err := ops.CueHealth(ctx, path)
	if err != nil {
		return false, fmt.Errorf("mkvgo has-seek-index: %w", err)
	}
	return ch.Healthy, nil
}

// rewriteHeadroomBytes is the safety margin demanded on top of the source
// size by the rewrite's space preflight - room for the container overhead of
// the rebuilt copy and for the rest of the system to keep breathing.
const rewriteHeadroomBytes = 256 << 20 // 256 MiB

// diskAvail reports the bytes available to an unprivileged user on the
// filesystem holding dir. Package-level so tests can simulate a full disk.
var diskAvail = func(dir string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, err
	}
	return st.Bavail * uint64(st.Bsize), nil //nolint:gosec // Bsize is a positive block size
}

// fmtBytes renders a byte count for operator-facing errors (GiB precision
// matters here, not exact bytes).
func fmtBytes(n uint64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := uint64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// permissionRemedy converts a raw EACCES from the repair machinery into the
// operator-actionable diagnosis: the server user does not own the media file
// (or its directory). Without this, a whole batch fails as N copies of a
// cryptic "permission denied" buried in strategy errors.
func permissionRemedy(err error) error {
	return fmt.Errorf("reindex: %w (server uid %d): set the container's PUID/PGID to the owner of your media files (docs/configuration.md, \"File ownership\") and retry: %w",
		mkv.ErrMediaNotWritable, os.Getuid(), err)
}

// removeQuiet deletes path best-effort. A missing file is fine (the temp may
// never have been created); any other failure is logged but never fatal - these
// are cleanup paths where the caller already holds its real result.
func removeQuiet(path string) {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) { //nolint:gosec // operator media path
		slog.Warn("mkvgo: temp cleanup failed", slog.String("path", path), slog.Any("err", err))
	}
}

// reindexResync repairs a corrupt file the strict reindex refused: the
// tolerant rewrite (ops.ReindexReplace with Resync) walks past the damage and
// resynchronises on the next valid cluster, keeping a backup. The skipped
// spans are collected and returned so the operator sees what was lost.
func (a *Adapter) reindexResync(ctx context.Context, path string) (mkv.ReindexResult, error) {
	srcInfo, err := os.Stat(path) //nolint:gosec // path is an operator's media file from the library catalog, not user input
	if err != nil {
		return mkv.ReindexResult{}, fmt.Errorf("reindex resync: stat source: %w", err)
	}
	var ranges []mkv.DamagedRange
	err = ops.ReindexReplace(ctx, path, extmkv.Options{
		Resync:     true,
		KeepBackup: true, // this repair drops bytes - a rollback copy is mandatory
		OnSkip: func(d extmkv.DamagedRange) {
			ranges = append(ranges, mkv.DamagedRange{
				StartOffset: d.StartOffset, EndOffset: d.EndOffset,
				ApproxStartMs: d.ApproxStartMs, ApproxEndMs: d.ApproxEndMs,
			})
		},
	})
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return mkv.ReindexResult{}, permissionRemedy(err)
		}
		return mkv.ReindexResult{}, fmt.Errorf("reindex resync: %w", err)
	}
	// Adopt Evey's backup convention so rollback/purge/scan see it.
	bak := path + backupSuffix
	if rerr := os.Rename(path+mkvgoReplaceBackupSuffix, bak); rerr != nil { //nolint:gosec // operator media path
		if !os.IsNotExist(rerr) {
			return mkv.ReindexResult{}, fmt.Errorf("reindex resync: adopt backup name: %w", rerr)
		}
		bak = "" // no backup produced (should not happen with KeepBackup)
	}
	dstBytes := srcInfo.Size()
	if di, dErr := os.Stat(path); dErr == nil { //nolint:gosec // operator media path
		dstBytes = di.Size()
	}
	return mkv.ReindexResult{
		Method:        mkv.ReindexMethodRewrite,
		BackupPath:    bak,
		SrcBytes:      srcInfo.Size(),
		DstBytes:      dstBytes,
		DamagedRanges: ranges,
	}, nil
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
func (a *Adapter) ReindexInPlace(ctx context.Context, path string, opts mkv.ReindexOptions) (mkv.ReindexResult, error) {
	ctx, span := tracing.StartSpan(ctx, "mkvgo.ReindexInPlace")
	defer span.End()

	if !IsEBMLContainer(path) {
		return mkv.ReindexResult{Skipped: true}, nil
	}

	// (0) Resync repair: the file was REFUSED by a strict reindex for a
	// corrupted region. Route straight to the tolerant rewrite - it walks past
	// the damage, resynchronises on the next valid cluster, and reports every
	// skipped span. Always rewrites (cannot drop bytes in place) and keeps a
	// backup (this repair loses the skipped bytes). Runs before the strict
	// meta read below, which itself can fail on such a file.
	if opts.Resync {
		return a.reindexResync(ctx, path)
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
		// Presence is not usability. A phantom index - an empty body, entries
		// keyed only on the audio track, a Cues element the SeekHead never
		// names - counts as "indexed" by a raw entry count while the scan's
		// judge (CueHealth) keeps flagging the file. That disagreement pinned
		// 129 production files in a sterile loop: flagged by every scan,
		// skipped as "already indexed" by every repair, forever.
		//
		// Skip only what the scan itself would call indexed, and take the
		// verdict WHOLE. Judging one component (`VideoCues > 0`) closed the
		// misskeyed case and left the loop open for "index-sparse" - video
		// cues that exist and are too far apart to seek into, so VideoCues > 0
		// holds by definition. Prod 2026-07-26: 138 files "repaired", 134
		// still flagged afterwards.
		ch, chErr := ops.CueHealth(ctx, path)
		if chErr != nil {
			return mkv.ReindexResult{}, fmt.Errorf("reindex: cue health: %w", chErr)
		}
		if ch.Healthy {
			return mkv.ReindexResult{Skipped: true}, nil // usable index - nothing to do
		}
	}

	// (2) Surgical in-place first (Mode auto/inplace): needs write access to
	// the FILE only (no temp in the directory), never duplicates it on disk,
	// leaves no backup. In ModeAuto it falls back to the full rewrite below
	// when the strategy is unavailable (pre-bump stub) or refuses this file
	// (streamed clusters, truncated tail, no SeekHead slot) - the in-place
	// journal guarantees the original is intact after any refusal, so the
	// rewrite can safely proceed. In ModeInPlace the operator explicitly
	// refused a rewrite: refusal is surfaced as ErrInPlaceUnsupported.
	// ModeRewrite never attempts the patch.
	if srcInfo, statErr := os.Stat(path); statErr != nil && opts.Mode == mkv.ModeInPlace { //nolint:gosec // operator media path
		return mkv.ReindexResult{}, fmt.Errorf("reindex: stat source: %w", statErr)
	} else if statErr == nil && opts.Mode != mkv.ModeRewrite {
		switch inErr := reindexSurgical(ctx, path, opts.DeepVerify); {
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
			if dstInfo, dErr := os.Stat(path); dErr == nil { //nolint:gosec // operator media path
				dstBytes = dstInfo.Size()
			}
			return mkv.ReindexResult{
				Method:   mkv.ReindexMethodInPlace,
				SrcBytes: srcInfo.Size(),
				DstBytes: dstBytes,
			}, nil
		case errors.Is(inErr, fs.ErrPermission) && opts.Mode == mkv.ModeInPlace:
			// Not a layout/build refusal: the server user may not write the
			// file. Name the ownership problem instead of ErrInPlaceUnsupported.
			// (ModeAuto still falls through below - the temp+rename rewrite
			// only needs DIRECTORY write and can rescue a read-only file.)
			return mkv.ReindexResult{}, permissionRemedy(inErr)
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

	// Tracks + DurationMs live in the head: no cluster walk for a cross-check.
	src, err := reader.OpenMeta(ctx, path)
	if err != nil {
		return mkv.ReindexResult{}, fmt.Errorf("reindex: open source: %w", err)
	}
	srcTracks := len(src.Tracks)
	srcDurMs := src.DurationMs

	srcInfo, err := os.Stat(path) //nolint:gosec // operator media path
	if err != nil {
		return mkv.ReindexResult{}, fmt.Errorf("reindex: stat source: %w", err)
	}

	// Space preflight: the rewrite writes a full sibling copy before the swap
	// (and with KeepBackup that extra copy PERSISTS as the .mkvgobak). Refuse
	// upfront with the numbers when the volume cannot hold it, instead of
	// filling a nearly-full disk mid-copy. Probe failures never block - the
	// guard is advisory, the copy itself remains the authority.
	if avail, aerr := diskAvail(filepath.Dir(path)); aerr == nil {
		need := uint64(srcInfo.Size()) + rewriteHeadroomBytes
		if avail < need {
			detail := "temporary copy of the file"
			if opts.KeepBackup {
				detail = "temporary copy of the file, kept as the backup afterwards"
			}
			return mkv.ReindexResult{}, fmt.Errorf(
				"reindex: %w: the rewrite needs ~%s free on this volume (%s), %s available - free up space, repair without backups, or use the in-place mode",
				mkv.ErrInsufficientSpace, fmtBytes(need), detail, fmtBytes(avail))
		}
	}

	// (2) Reindex to a sibling temp. Same directory → same filesystem, so the
	// final swap is a true atomic rename.
	tmp := path + ".reindex.tmp"
	removeQuiet(tmp) // clear any stale temp from a previous aborted run
	if err = ops.Reindex(ctx, path, tmp, extmkv.Options{DeepVerify: opts.DeepVerify}); err != nil {
		removeQuiet(tmp)
		if errors.Is(err, fs.ErrPermission) {
			// Neither the file (in-place above) nor its directory (this temp)
			// is writable by the server user: pure ownership problem.
			return mkv.ReindexResult{}, permissionRemedy(err)
		}
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
	out, err := reader.OpenMeta(ctx, tmp)
	if err != nil {
		return mkv.ReindexResult{}, fmt.Errorf("reindex: re-open output: %w", err)
	}
	if len(out.Tracks) != srcTracks {
		return mkv.ReindexResult{}, fmt.Errorf("reindex: track count changed (%d -> %d), refusing swap", srcTracks, len(out.Tracks))
	}
	// Same rule as the bail-out: read the Cues element's real content to prove
	// the rebuilt FILE carries an index.
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

	tmpInfo, err := os.Stat(tmp) //nolint:gosec // operator media path
	if err != nil {
		return mkv.ReindexResult{}, fmt.Errorf("reindex: stat output: %w", err)
	}

	// (5) Atomic swap with a recoverable backup.
	bak := path + backupSuffix
	if err := os.Rename(path, bak); err != nil { //nolint:gosec // operator media path
		return mkv.ReindexResult{}, fmt.Errorf("reindex: back up original: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil { //nolint:gosec // operator media path
		if rerr := os.Rename(bak, path); rerr != nil { //nolint:gosec // operator media path
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
		if err := os.Remove(bak); err != nil { //nolint:gosec // operator media path
			slog.Warn("mkvgo: reindex backup cleanup failed (left in place)",
				slog.String("backup", bak), slog.Any("err", err))
		} else {
			res.BackupPath = ""
		}
	}
	return res, nil
}
