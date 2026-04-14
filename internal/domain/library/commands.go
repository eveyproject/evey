package library

import (
	"context"

	"github.com/gravity-zero/evey/internal/domain/shared"
)

type LibraryCommandRepo interface {
	Create(ctx context.Context, lib *Library) error
	Update(ctx context.Context, lib *Library) error
	Delete(ctx context.Context, id shared.LibraryID) error
}

type MediaItemCommandRepo interface {
	Create(ctx context.Context, item *MediaItem) error
	Update(ctx context.Context, item *MediaItem) error
	Delete(ctx context.Context, id shared.MediaItemID) error
	DeleteByIDs(ctx context.Context, ids []string) error
	GetByID(ctx context.Context, id shared.MediaItemID) (*MediaItem, error)
	GetByFilePath(ctx context.Context, path string) (*MediaItem, error)
	GetByFileFingerprint(ctx context.Context, fingerprint string) (*MediaItem, error)
	GetByLibraryTypeTitle(ctx context.Context, libraryID shared.LibraryID, itemType, title, parentID string) (*MediaItem, error)
	// ListFilePaths returns id→filePath for all items in a library (for orphan detection).
	ListFilePaths(ctx context.Context, libraryID shared.LibraryID) (map[string]string, error)
	ListByLibraryID(ctx context.Context, libraryID shared.LibraryID) ([]*MediaItem, error)
	ListScanSummaries(ctx context.Context, libraryID shared.LibraryID) ([]MediaItemScanRow, error)
	ListContainerInfo(ctx context.Context, libraryID shared.LibraryID) ([]MediaItemContainerRow, error)
}
