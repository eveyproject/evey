package library

import "context"

type LibraryCommandRepo interface {
	Create(ctx context.Context, lib *Library) error
	Update(ctx context.Context, lib *Library) error
	Delete(ctx context.Context, id string) error
}

type MediaItemCommandRepo interface {
	Create(ctx context.Context, item *MediaItem) error
	Update(ctx context.Context, item *MediaItem) error
	Delete(ctx context.Context, id string) error
	DeleteByIDs(ctx context.Context, ids []string) error
	GetByID(ctx context.Context, id string) (*MediaItem, error)
	GetByFilePath(ctx context.Context, path string) (*MediaItem, error)
	GetByFileFingerprint(ctx context.Context, fingerprint string) (*MediaItem, error)
	GetByLibraryTypeTitle(ctx context.Context, libraryID, itemType, title, parentID string) (*MediaItem, error)
	// ListFilePaths returns id→filePath for all items in a library (for orphan detection).
	ListFilePaths(ctx context.Context, libraryID string) (map[string]string, error)
	ListByLibraryID(ctx context.Context, libraryID string) ([]*MediaItem, error)
	ListScanSummaries(ctx context.Context, libraryID string) ([]MediaItemScanRow, error)
	ListContainerInfo(ctx context.Context, libraryID string) ([]MediaItemContainerRow, error)
}
