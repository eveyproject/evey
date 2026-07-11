// Package scanindex provides the shared change-detection primitives used
// by every library scanner (video/TV, music, photo). All three flows
// want the same sequence: build an index of existing rows keyed by path
// and content hash, then decide per-file whether the row is unchanged,
// moved, or needs reprocessing. Keeping this in one place removes three
// copies of the same ~25-line block and unifies the ±1µs Postgres mtime
// rounding workaround that every site was reinventing.
package scanindex

import "time"

// SameMtime treats two filesystem timestamps as equal when they match at
// Postgres-storage precision, accepting a ±1µs rounding drift. The
// filesystem exposes ns-level mtimes (ext4, xfs, apfs) while Postgres
// TIMESTAMPTZ rounds sub-µs to µs, and the direction (half-up vs
// half-even) varies between values/drivers. Both Truncate and Round can
// therefore miss pairs that differ by one µs after round-trip, which
// previously flagged every file as "changed" each scan and pinned CPU
// at 100%. The ±1µs window absorbs the drift without masking genuine
// edits - real content changes always move mtime by ≥1 second.
func SameMtime(a, b time.Time) bool {
	at := a.Truncate(time.Microsecond)
	bt := b.Truncate(time.Microsecond)
	if at.Equal(bt) {
		return true
	}
	// Check both directions without negating a potentially-saturated
	// Duration (zero-vs-now.Sub overflows to MinInt64 which -x can't
	// un-wrap).
	return at.Sub(bt) == time.Microsecond || bt.Sub(at) == time.Microsecond
}

// Summary is the projection of an existing DB row needed for
// change-detection. Each caller maps its own domain type to this.
type Summary struct {
	ID      string
	Path    string
	Size    int64
	ModTime time.Time
	Hash    string // "" when the row has no stored hash
	// NeedsReprobe marks a row the scanner must reprocess even when it is
	// otherwise unchanged - e.g. a video file indexed without streams that is
	// owed a bounded re-probe. Callers that don't track this leave it false.
	NeedsReprobe bool
}

// Index keeps existing rows indexed by path and (when known) by hash,
// ready for per-file lookup during a scan pass.
type Index struct {
	byPath map[string]Summary
	byHash map[string]Summary
}

// New builds an Index from a slice of summaries. Hash entries are
// inserted only when the Summary.Hash field is non-empty.
func New(summaries []Summary) *Index {
	idx := &Index{
		byPath: make(map[string]Summary, len(summaries)),
		byHash: make(map[string]Summary, len(summaries)),
	}
	for _, s := range summaries {
		idx.byPath[s.Path] = s
		if s.Hash != "" {
			idx.byHash[s.Hash] = s
		}
	}
	return idx
}

// Unchanged reports whether an incoming file matches an existing row at
// the same path with identical size and mtime (±1µs). Fast path - no
// hash computation required. Returns the existing ID when a row exists
// at the path regardless of match, so callers can correlate.
func (idx *Index) Unchanged(path string, size int64, modTime time.Time) (existingID string, unchanged bool) {
	ex, found := idx.byPath[path]
	if !found {
		return "", false
	}
	if ex.Size != size || !SameMtime(ex.ModTime, modTime) {
		return ex.ID, false
	}
	return ex.ID, true
}

// NeedsReprobe reports whether the row at the given path is flagged for
// reprocessing despite being otherwise unchanged. False when no row exists.
func (idx *Index) NeedsReprobe(path string) bool {
	ex, ok := idx.byPath[path]
	return ok && ex.NeedsReprobe
}

// ExistingHash returns the hash persisted for the row at the given
// path, and whether such a row exists. Callers that have a
// pre-computed hash at fast-path time (video) use this to rule out
// inode/mtime coincidences where the OS reused both values for
// different content.
func (idx *Index) ExistingHash(path string) (hash string, found bool) {
	ex, ok := idx.byPath[path]
	if !ok {
		return "", false
	}
	return ex.Hash, true
}

// Moved returns the existing ID of a row whose stored hash matches the
// incoming hash, provided no row already exists at the incoming path.
// This flags a rename/relocation - the caller updates the path in
// place instead of reprocessing content. Empty hash never matches.
func (idx *Index) Moved(incomingPath, hash string) (existingID string, moved bool) {
	if hash == "" {
		return "", false
	}
	if _, hasAtPath := idx.byPath[incomingPath]; hasAtPath {
		return "", false
	}
	ex, found := idx.byHash[hash]
	if !found {
		return "", false
	}
	return ex.ID, true
}
