package library

import (
	"context"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
)

type LibraryCommandRepo interface {
	Create(ctx context.Context, lib *Library) error
	Update(ctx context.Context, lib *Library) error
	Delete(ctx context.Context, id shared.LibraryID) error
	// TouchLastScannedAt records when a scan ran and writes nothing else: a
	// scan holds a copy of the library read at its start, and writing that
	// copy back would undo an edit the admin saved while it ran.
	TouchLastScannedAt(ctx context.Context, id shared.LibraryID, at time.Time) error
}

// MediaItemCommandRepo is the write port for media items, per ADR-0009. It
// still carries a handful of read methods on purpose: each one is consumed
// exclusively by the scanner's write pipeline (ScanService/OrphanPruner) to
// decide what to Create/Update/Delete in the very same call, never by a
// read-only caller. A read whose only reason to exist is gating a command
// in the same function is not a CQRS violation - see ADR-0009's "read
// consumed by a command" allowance. Reads with independent, read-only
// callers live on MediaItemQueryRepo instead (queries.go).
type MediaItemCommandRepo interface {
	Create(ctx context.Context, item *MediaItem) error
	Update(ctx context.Context, item *MediaItem) error
	Delete(ctx context.Context, id shared.MediaItemID) error
	DeleteByIDs(ctx context.Context, ids []string) error
	// GetByID resolves the moved-file candidate scan.detectChanges just
	// matched, so it can Update the existing item's path in place.
	GetByID(ctx context.Context, id shared.MediaItemID) (*MediaItem, error)
	// GetByFilePath gates scan.findExistingMediaFile's Create: does this file
	// already own a row before we insert a new one?
	GetByFilePath(ctx context.Context, path string) (*MediaItem, error)
	// GetByFileFingerprint is findExistingMediaFile's move/rename fallback
	// when the path lookup above misses - same insert-gating purpose.
	GetByFileFingerprint(ctx context.Context, fingerprint string) (*MediaItem, error)
	// ListFilePaths returns id→filePath for all items in a library, feeding
	// OrphanPruner.Prune's decision of which rows to DeleteByIDs.
	ListFilePaths(ctx context.Context, libraryID shared.LibraryID) (map[string]string, error)
	// ListByLibraryID is EVERY item of a library, whatever its size: the
	// television scan preloads it to match each file against the rows it
	// has, and ScanService.healOrphanTitles Updates any item whose stored
	// title drifted from a fresher parser pass. An adapter that cut it short
	// made the preload blind to the rows past the cut, so their files were
	// created again until the unique fingerprint refused the scan.
	ListByLibraryID(ctx context.Context, libraryID shared.LibraryID) ([]*MediaItem, error)
	// FindReplacedFile is the item a file found under a NEW name stands in
	// for: an episode of the same season with the same number, or a movie of
	// the same folder with the same parsed title and year, whose own path is
	// not f.Path. A quality upgrade by a download manager lands as a new
	// name and a new size at once, so neither the path nor the fingerprint
	// finds the row - and creating a second one let the prune delete the
	// first with every viewer's progress, favourites and playlists cascading
	// after it. ErrNotFound when no such item exists; the caller still checks
	// the candidate's own file is gone before repointing it.
	FindReplacedFile(ctx context.Context, libraryID shared.LibraryID, parentID string, f ScannedFile) (*MediaItem, error)
	// ListScanSummaries seeds the scan-pass diff index (scanindex.Index) that
	// drives every Create/Update decision for the rest of the scan.
	ListScanSummaries(ctx context.Context, libraryID shared.LibraryID) ([]MediaItemScanRow, error)
	// ListContainerInfo backs OrphanPruner.pruneEmptyContainers's decision of
	// which empty season/series rows to Delete.
	ListContainerInfo(ctx context.Context, libraryID shared.LibraryID) ([]MediaItemContainerRow, error)
}
