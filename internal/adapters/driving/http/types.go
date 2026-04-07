package httpapi

import (
	"context"

	domainlibrary "github.com/gravity-zero/evey/internal/domain/library"
	domainmeta "github.com/gravity-zero/evey/internal/domain/metadata"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
)

// LibraryAccessChecker checks whether a user has access to a library.
type LibraryAccessChecker interface {
	HasAccess(ctx context.Context, userID, libraryID string) (bool, error)
}

// libraryPathsGetter retrieves a library with its configured paths.
type libraryPathsGetter interface {
	GetLibrary(ctx context.Context, id string) (*domainlibrary.Library, error)
}

// mediaLibraryGetter fetches a media item to retrieve its LibraryID.
type mediaLibraryGetter interface {
	GetMediaItem(ctx context.Context, id string) (*domainlibrary.MediaItem, error)
}

// remoteAccessPersister persists remote-access configuration.
type remoteAccessPersister interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
}

// TokenVerifier validates access tokens and returns claims.
type TokenVerifier interface {
	ValidateAccessToken(token string) (*domainuser.Claims, error)
}

// UserMaxStreamsGetter returns the MaxStreams limit for a given user ID (nil = unlimited).
type UserMaxStreamsGetter interface {
	GetMaxStreams(ctx context.Context, userID string) *int
}

// UserBitrateGetter returns the max streaming bitrate (kbps) for a user. 0 = unlimited.
type UserBitrateGetter interface {
	GetUserBitrateLimit(ctx context.Context, userID string) (int, error)
}

// MediaItemGetter retrieves a single media item by ID.
type MediaItemGetter interface {
	GetMediaItem(ctx context.Context, id string) (*domainlibrary.MediaItem, error)
}

// MediaStreamsGetter retrieves streams for a media item.
type MediaStreamsGetter interface {
	GetMediaItem(ctx context.Context, id string) (*domainlibrary.MediaItem, error)
	GetMediaStreams(ctx context.Context, mediaItemID string) ([]*domainlibrary.MediaStream, error)
}

// ChaptersGetter retrieves chapters for a media item.
type ChaptersGetter interface {
	GetChapters(ctx context.Context, mediaItemID string) ([]*domainlibrary.Chapter, error)
}

// TrickplayGetter retrieves trickplay manifests.
type TrickplayGetter interface {
	GetTrickplayManifest(ctx context.Context, mediaItemID string) (*domainlibrary.TrickplayManifest, error)
}

// TrickplayGenerator generates trickplay thumbnails.
type TrickplayGenerator interface {
	GenerateTrickplay(ctx context.Context, mediaItemID string) error
}

// AllTrickplayGenerator generates all missing trickplay thumbnails.
type AllTrickplayGenerator interface {
	GenerateAllMissingTrickplays(ctx context.Context) (int, error)
}

// FrameExtractor extracts a single video frame at a given position.
type FrameExtractor interface {
	ExtractFrame(ctx context.Context, filePath string, positionSec float64, width int) ([]byte, error)
}

// ArtworkRepo reads and updates artwork records.
type ArtworkRepo interface {
	GetByMediaItem(ctx context.Context, mediaItemID string) ([]*domainmeta.ArtworkRecord, error)
	UpdateSortOrder(ctx context.Context, id string, sortOrder int) error
	UpdateWebPPath(ctx context.Context, id string, webpPath string) error
}

// ArtworkReader reads artwork records (read-only subset).
type ArtworkReader interface {
	GetByMediaItem(ctx context.Context, mediaItemID string) ([]*domainmeta.ArtworkRecord, error)
}

// ImageConverter converts images to WebP.
type ImageConverter interface {
	ConvertToWebP(ctx context.Context, srcPath, dstPath string, quality int) error
}

// LibraryLister lists all libraries.
type LibraryLister interface {
	ListLibraries(ctx context.Context) ([]*domainlibrary.Library, error)
}
