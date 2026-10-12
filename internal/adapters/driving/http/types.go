package httpapi

import (
	"context"
	"github.com/eveyproject/evey/internal/pkg/auth"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	domainmeta "github.com/eveyproject/evey/internal/domain/metadata"
	"github.com/eveyproject/evey/internal/domain/shared"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
)

// LibraryAccessChecker checks whether a user has access to a library.
type LibraryAccessChecker interface {
	HasAccess(ctx context.Context, userID shared.UserID, libraryID shared.LibraryID) (bool, error)
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
	// Delete removes a persisted override entirely - the settings panel's
	// Reset must not leave a row that a restart would re-apply.
	Delete(ctx context.Context, key string) error
}

// TokenVerifier validates access tokens and returns claims.
type TokenVerifier interface {
	ValidateAccessToken(token string) (*domainuser.Claims, error)
}

// PluginTokenParser validates scoped plugin tokens (plugin iframes send them
// instead of user access tokens). Satisfied by the raw *auth.JWTGenerator -
// plugin tokens carry no user epoch, so they bypass the epoch guard on purpose.
type PluginTokenParser interface {
	ParsePluginToken(raw string) (*auth.PluginClaims, error)
}

// UserMaxStreamsGetter returns the MaxStreams limit for a given user ID (nil = unlimited).
type UserMaxStreamsGetter interface {
	GetMaxStreams(ctx context.Context, userID string) *int
}

// UserBitrateGetter returns the max streaming bitrate (kbps) for a user. 0 = unlimited.
type UserBitrateGetter interface {
	GetUserBitrateLimit(ctx context.Context, userID string) (int, error)
}

// ProfilePrefsGetter returns the profile.preferences (JSONB) map for a given profile ID.
// Used to look up user transcoding prefs (preferred_video_codec, allow_direct_play, etc.).
type ProfilePrefsGetter interface {
	GetProfilePreferences(ctx context.Context, profileID string) (map[string]string, error)
}

// MediaItemGetter retrieves a single media item by ID.
type MediaItemGetter interface {
	GetMediaItem(ctx context.Context, id string) (*domainlibrary.MediaItem, error)
}

// MediaArchiveSource retrieves a media item, lists its children, and lists
// libraries. The download handler uses it to assemble a ZIP for container
// items (a season's episodes, or every episode of a series) and to confine the
// files it bundles to the configured library roots.
type MediaArchiveSource interface {
	GetMediaItem(ctx context.Context, id string) (*domainlibrary.MediaItem, error)
	GetMediaItemsByIDs(ctx context.Context, ids []string) ([]*domainlibrary.MediaItem, error)
	ListMediaItems(ctx context.Context, filter domainlibrary.ListMediaItemsFilter) ([]*domainlibrary.MediaItem, error)
	ListLibraries(ctx context.Context) ([]*domainlibrary.Library, error)
}

// MediaStreamsGetter retrieves streams for a media item.
type MediaStreamsGetter interface {
	GetMediaItem(ctx context.Context, id string) (*domainlibrary.MediaItem, error)
	GetMediaStreams(ctx context.Context, mediaItemID string) ([]*domainlibrary.MediaStream, error)
	// NextEpisodeOf resolves the next episode in the same series - same
	// season's next index, or first ep of the next season. nil + nil err
	// when current is the series finale. Consumed by the predictive
	// warm-up endpoint.
	NextEpisodeOf(ctx context.Context, episodeID string) (*domainlibrary.MediaItem, error)
}

// ChaptersGetter retrieves chapters for a media item.
type ChaptersGetter interface {
	GetChapters(ctx context.Context, mediaItemID string) ([]*domainlibrary.Chapter, error)
}

// SegmentsGetter retrieves media segments (intro/outro/etc.) for a media item.
type SegmentsGetter interface {
	GetSegments(ctx context.Context, mediaItemID string) ([]*domainlibrary.MediaSegment, error)
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

// KeyframeExtractor pulls the encoded video keyframe nearest a timestamp
// straight from the container's seek index (mkvgo, bounded reads - no full-file
// scan, no decode). Returns the packet bytes, a codec extension
// (".h264"/".hevc"/".ivf") a decoder needs, and the keyframe's real PTS (so the
// caller can reject a too-distant keyframe and fall back to an exact decode).
// Errors when the source can't be seeked this way (no Cues, unsupported
// container), signalling the ffmpeg fallback.
type KeyframeExtractor interface {
	ExtractKeyframePacket(ctx context.Context, filePath string, atMs int64) (packet []byte, ext string, ptsMs int64, err error)
}

// FramePacketDecoder decodes one encoded keyframe packet to a JPEG (ffmpeg, a
// single frame), scaled to width. Companion to KeyframeExtractor: mkvgo seeks +
// extracts the packet, ffmpeg decodes just that one frame instead of the whole
// GOP up to the requested position.
type FramePacketDecoder interface {
	DecodePacketToJPEG(ctx context.Context, packet []byte, ext string, width int) ([]byte, error)
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

// ImageConverter converts images to WebP, with optional resize.
type ImageConverter interface {
	ConvertToWebP(ctx context.Context, srcPath, dstPath string, quality int) error
	// ResizeAndConvertToWebP scales src to fit width (height auto, ratio preserved)
	// and encodes WebP at quality. width=0 means no resize.
	ResizeAndConvertToWebP(ctx context.Context, srcPath, dstPath string, width, quality int) error
}

// LibraryLister lists all libraries.
type LibraryLister interface {
	ListLibraries(ctx context.Context) ([]*domainlibrary.Library, error)
}

// IntroDetectionTrigger triggers intro/outro detection for a media item and
// exposes manual override operations for admin use.
type IntroDetectionTrigger interface {
	DetectIntro(ctx context.Context, mediaItemID string) error
	DetectIntroDryRun(ctx context.Context, mediaItemID string) ([]domainlibrary.MediaSegment, error)
	// DetectSeason / DetectSeries run the SEASON pipeline (cluster included)
	// on one season, or on every season of a series - the admin intro-test
	// tool's "apply" granularity. DetectSeries returns the season count.
	DetectSeason(ctx context.Context, seasonID string) error
	DetectSeries(ctx context.Context, seriesID string) (int, error)
	BackfillLibrary(ctx context.Context, libraryID string) (int, error)
	SetManualSegment(ctx context.Context, mediaItemID, segType string, startMs, endMs int64) error
	DeleteSegment(ctx context.Context, mediaItemID, segType string) error
	PropagateSegment(ctx context.Context, refMediaItemID, segType string) ([]string, error)
}
