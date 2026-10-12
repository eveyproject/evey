package library

import (
	"context"

	"github.com/eveyproject/evey/internal/domain/shared"
)

type LibraryCommandRepo interface {
	Create(ctx context.Context, lib *Library) error
	Update(ctx context.Context, lib *Library) error
	Delete(ctx context.Context, id shared.LibraryID) error
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
	// ListByLibraryID backs ScanService.healOrphanTitles, which Updates any
	// item whose stored title drifted from a fresher parser pass.
	ListByLibraryID(ctx context.Context, libraryID shared.LibraryID) ([]*MediaItem, error)
	// ListScanSummaries seeds the scan-pass diff index (scanindex.Index) that
	// drives every Create/Update decision for the rest of the scan.
	ListScanSummaries(ctx context.Context, libraryID shared.LibraryID) ([]MediaItemScanRow, error)
	// ListContainerInfo backs OrphanPruner.pruneEmptyContainers's decision of
	// which empty season/series rows to Delete.
	ListContainerInfo(ctx context.Context, libraryID shared.LibraryID) ([]MediaItemContainerRow, error)
}
