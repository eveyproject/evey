package mkv

import (
	"context"
	"errors"
)

// ErrNoBackup is returned by RestoreBackup/DeleteBackup when path has no
// "<path>.mkvgobak" - callers map it to a clean not-found instead of a 500.
var ErrNoBackup = errors.New("no reindex backup")

// Reindexer rebuilds the seek index (Cues) of an EBML container so a player can
// seek/resume in O(1) instead of linearly scanning a file muxed without an
// index - the root cause of slow resume on such files. The capability writes
// into the source file, so the implementation MUST be fail-safe: whatever the
// strategy, a failure leaves the original byte-for-byte intact.
type Reindexer interface {
	// HasSeekIndex reports whether the EBML file at path already carries a Cues
	// seek index. Non-EBML files report (false, nil) - there is nothing to index.
	HasSeekIndex(ctx context.Context, path string) (bool, error)

	// ReindexInPlace rebuilds path's seek index safely. Two strategies, chosen
	// by the implementation (ReindexResult.Method reports which ran):
	//
	//   - "inplace": surgically patch the file itself (append a fresh Cues,
	//     extend the Segment, repoint the SeekHead) with an in-file journal
	//     making the operation crash-safe. Needs write access to the FILE
	//     only, never duplicates it on disk, and leaves NO backup - the
	//     journal is truncated away on success.
	//   - "rewrite": write a reindexed copy to a sibling temp file, fully
	//     re-parse + validate it, cross-check it against the original (track
	//     count, duration, a freshly-present index), then atomically swap it
	//     in, keeping the original as a backup per opts. The fallback for
	//     files the surgical patch refuses (streamed clusters, truncated
	//     tails, no SeekHead slot).
	//
	// On ANY failure the original file is left byte-for-byte intact. Returns
	// Skipped=true when the file already has an index or is not an EBML
	// container.
	ReindexInPlace(ctx context.Context, path string, opts ReindexOptions) (ReindexResult, error)

	// HasBackup reports whether a reindex backup ("<path>.mkvgobak") exists, i.e.
	// whether path can be rolled back to its pre-reindex state.
	HasBackup(ctx context.Context, path string) (bool, error)

	// RestoreBackup undoes a prior reindex by atomically replacing path with its
	// "<path>.mkvgobak" backup. Returns an error if no backup exists.
	RestoreBackup(ctx context.Context, path string) error

	// DeleteBackup removes the "<path>.mkvgobak" backup once the operator has
	// confirmed the repaired file plays fine - reclaiming the duplicated disk
	// space is the point. Returns an error if no backup exists.
	DeleteBackup(ctx context.Context, path string) error

	// SupportsInPlace reports whether the surgical in-place strategy is
	// available in this build (it activates with the mkvgo release shipping
	// ReindexInPlace). The UI uses it to offer the mode only when real.
	SupportsInPlace() bool
}

// ErrInPlaceUnsupported is returned by ReindexInPlace when the caller demands
// ModeInPlace but the strategy is unavailable (pre-bump build) or the file is
// refused by it (streamed clusters, truncated tail, no SeekHead slot). The
// operator can retry with ModeRewrite.
var ErrInPlaceUnsupported = errors.New("in-place reindex not supported for this file or build")

// ErrMediaNotWritable is returned when a repair fails because the server's
// user may not write the media file (or its directory) - an ownership problem
// the operator fixes by aligning the container's PUID/PGID with the media
// owner, not a defect of the file or the build. Kept distinct so callers can
// aggregate a whole batch of these into one actionable message.
var ErrMediaNotWritable = errors.New("media not writable by the server user")

// ErrInsufficientSpace is returned when a rewrite repair is refused upfront
// because the media volume cannot hold the temporary sibling copy the rewrite
// swaps in. Preflighting keeps a batch from filling a nearly-full disk and
// starving playback/neighbors mid-copy.
var ErrInsufficientSpace = errors.New("not enough free disk space on the media volume for a rewrite")

// ErrCorruptSource is returned when the strict rewrite refuses a file because
// its cluster stream does not parse: a corrupted region a RESYNC repair can
// skip past, at the cost of the bytes it drops. It is the one repair failure
// that names its own remedy, and the scan cannot anticipate it - the scan is
// head-mostly on purpose (a 7 000-file library must not each cost a full
// salvage walk on a NAS), so this damage is only ever discovered BY a repair.
// Callers record it on the file instead of discarding it with the error.
var ErrCorruptSource = errors.New("the source is structurally corrupt")

// Reindex modes the operator can demand (ReindexOptions.Mode).
const (
	// ModeAuto (empty string): surgical in-place when available, otherwise the
	// rewrite fallback - the safest default.
	ModeAuto = ""
	// ModeInPlace: surgical patch ONLY; fail with ErrInPlaceUnsupported instead
	// of silently rewriting (the operator explicitly refused a duplicate).
	ModeInPlace = "inplace"
	// ModeRewrite: full rewrite + swap ONLY (never attempts the patch); the
	// KeepBackup option applies.
	ModeRewrite = "rewrite"
)

// ReindexResult.Method values.
const (
	// ReindexMethodInPlace: the file was surgically patched (no backup exists).
	ReindexMethodInPlace = "inplace"
	// ReindexMethodRewrite: the file was rebuilt to a copy and swapped in.
	ReindexMethodRewrite = "rewrite"
)

// ReindexOptions tunes the safe-replace behaviour.
type ReindexOptions struct {
	// Mode selects the strategy: ModeAuto (default), ModeInPlace or
	// ModeRewrite. See the constants for the exact contract.
	Mode string

	// KeepBackup retains the "<path>.mkvgobak" copy of the original after a
	// successful REWRITE swap, enabling a manual rollback even against a latent
	// bug that slipped past validation. When false, the backup is removed once
	// the swap commits. The in-place strategy never produces a backup and
	// ignores this flag.
	KeepBackup bool

	// DeepVerify makes mkvgo prove the rebuilt index against the real media
	// before committing: every cue point is checked to land on an actual video
	// keyframe (a full read of the file), on top of the always-on head-only
	// round-trip check. A failure aborts the rewrite before the swap, and rolls
	// the in-place patch back from its journal - so the original is intact
	// either way. Costs one full read; the in-place strategy keeps NO backup,
	// which is exactly why an operator may want it there.
	DeepVerify bool

	// Resync repairs a file the strict reindex REFUSED for a corrupted region
	// (junk between clusters, a size that lies): the tolerant rewrite skips the
	// damaged span and resynchronises on the next valid cluster, recovering
	// everything before and after. It ALWAYS rewrites (no in-place patch - you
	// cannot drop bytes in place) and forces a backup: this is the one repair
	// that loses data (the skipped span). The lost spans are reported in
	// ReindexResult.DamagedRanges.
	Resync bool
}

// DamagedRange is one span of the source a Resync repair could not carry over
// (junk/lying size), with the approximate presentation time it maps to - what
// the operator loses. Mirrors mkvgo's ops.DamagedRange, kept in the domain so
// the app layer doesn't import the adapter's dependency.
type DamagedRange struct {
	StartOffset   int64
	EndOffset     int64
	ApproxStartMs int64
	ApproxEndMs   int64
}

// ReindexResult reports the outcome of a ReindexInPlace call.
type ReindexResult struct {
	Skipped    bool     // already indexed or not an EBML container - no-op
	Method     string   // ReindexMethodInPlace | ReindexMethodRewrite ("" when skipped)
	BackupPath string   // retained backup path ("" for in-place / not kept / skipped)
	Warnings   []string // non-fatal validation warnings on the rebuilt file
	SrcBytes   int64
	DstBytes   int64
	// DamagedRanges lists the spans a Resync repair skipped (empty otherwise).
	DamagedRanges []DamagedRange
}
