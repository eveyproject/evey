package mkv

import "context"

// Reindexer rebuilds the seek index (Cues) of an EBML container so a player can
// seek/resume in O(1) instead of linearly scanning a file muxed without an
// index — the root cause of slow resume on such files. The capability rewrites
// the source file, so the implementation MUST be fail-safe: validate the rebuilt
// file and never lose the original.
type Reindexer interface {
	// HasSeekIndex reports whether the EBML file at path already carries a Cues
	// seek index. Non-EBML files report (false, nil) — there is nothing to index.
	HasSeekIndex(ctx context.Context, path string) (bool, error)

	// ReindexInPlace rebuilds path's seek index safely: it writes a reindexed
	// copy to a temp file, fully re-parses + validates it, cross-checks it against
	// the original (track count, duration, presence of a new index), then
	// atomically swaps it in while keeping the original as a backup. On ANY
	// failure the original file is left byte-for-byte untouched. Returns
	// Skipped=true when the file already has an index or is not an EBML container.
	ReindexInPlace(ctx context.Context, path string, opts ReindexOptions) (ReindexResult, error)
}

// ReindexOptions tunes the safe-replace behaviour.
type ReindexOptions struct {
	// KeepBackup retains the "<path>.mkvgobak" copy of the original after a
	// successful swap, enabling a manual rollback even against a latent bug that
	// slipped past validation. When false, the backup is removed once the swap
	// commits.
	KeepBackup bool
}

// ReindexResult reports the outcome of a ReindexInPlace call.
type ReindexResult struct {
	Skipped    bool     // already indexed or not an EBML container — no-op
	BackupPath string   // retained backup path ("" when not kept / skipped)
	Warnings   []string // non-fatal validation warnings on the rebuilt file
	SrcBytes   int64
	DstBytes   int64
}
