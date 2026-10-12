package metadata

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/google/uuid"

	"github.com/eveyproject/evey/internal/domain/metadata"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// command_service_artwork_custom.go - an admin supplying their own poster or
// backdrop, instead of picking one of the provider's.
//
// The gap it fills: every artwork on an item comes from TMDB/fanart, so a title
// they have nothing usable for (a home video, a rare edition, a bad match) is
// stuck with whatever was found or with nothing. The picker could only choose
// among existing rows.
//
// Nothing new is invented for storage. An ArtworkRecord already carries
// LocalPath and an optional SourceURL, and artworkDestPath already handles the
// no-source case by naming the file after the record id - so a custom image is
// an ordinary record whose provider happens to be us, not a special kind of
// object the rest of the pipeline has to know about. Serving, resizing,
// variants and the picker all keep working untouched.

// CustomArtworkProvider is the provider name recorded for admin uploads. It has
// to be distinguishable: re-enrichment deletes and re-creates provider rows, and
// an image the admin chose deliberately must not be swept away with them.
const CustomArtworkProvider = "custom"

// maxCustomArtworkBytes bounds one upload. Posters and backdrops are a few
// hundred KB; 12 MB leaves room for a lossless scan without letting the data
// volume be filled one request at a time.
const maxCustomArtworkBytes = 12 << 20

var errArtworkStorageDisabled = errors.New("artwork storage is not configured (no data dir)")

// AddCustomArtwork stores an admin-supplied image and makes it the item's
// current one.
//
// artType is a domain artwork type ("ARTWORK_TYPE_PRIMARY" / "..._BACKDROP").
// ext is the file extension to store under, already validated by the caller
// against the image's real content type - this function does not sniff bytes,
// because the HTTP layer has to do it anyway to reject non-images early.
//
// The new record is ranked below every existing one of its type, which is how
// this codebase says "this is the one" (the select endpoint promotes by writing
// a lower SortOrder): uploading IS choosing. An admin who uploads a poster and
// then has to pick it from a list would rightly wonder what the upload did.
func (s *CommandService) AddCustomArtwork(
	ctx context.Context,
	mediaItemID shared.MediaItemID,
	artType string,
	src io.Reader,
	ext string,
) (*metadata.ArtworkRecord, error) {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.AddCustomArtwork")
	defer span.End()

	dataDir := s.getDataDir()
	if dataDir == "" {
		return nil, errArtworkStorageDisabled
	}
	if mediaItemID == "" {
		return nil, errors.New("AddCustomArtwork: empty media item id")
	}
	if ext == "" {
		ext = ".jpg"
	}

	// Rank it below every existing artwork of this type rather than pinning it
	// to a constant: the select endpoint promotes by writing -1, so a previously
	// chosen image already sits there and a second -1 would be a tie broken by
	// nothing. One row lower is unambiguous, and needs no write to the others -
	// which matters because demoting them is not in this layer's repository
	// interface.
	sortOrder := -1
	if existing, err := s.artworkQuery.GetByMediaItem(ctx, string(mediaItemID)); err == nil {
		for _, other := range existing {
			if other != nil && other.Type == artType && other.SortOrder <= sortOrder {
				sortOrder = other.SortOrder - 1
			}
		}
	}

	rec := &metadata.ArtworkRecord{
		ID:          uuid.New().String(),
		MediaItemID: mediaItemID,
		Type:        artType,
		Provider:    CustomArtworkProvider,
		// SourceURL stays nil: there is no remote to re-download from, and it is
		// also what makes artworkDestPath name the file after the record id.
		SortOrder: sortOrder,
	}
	if err := rec.Validate(); err != nil {
		return nil, fmt.Errorf("AddCustomArtwork: %w", err)
	}

	dest := filepath.Join(dataDir, "artworks", string(mediaItemID), rec.ID+ext)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("AddCustomArtwork: create dir: %w", err)
	}

	// Write to a temp file and rename: a half-written poster served to a client
	// is worse than no poster, and the artwork server reads by path with no
	// notion of "still uploading".
	tmp := dest + ".part"
	f, err := os.Create(tmp) //nolint:gosec // dest is built from the data dir and a generated id
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("AddCustomArtwork: create file: %w", err)
	}
	written, copyErr := io.Copy(f, io.LimitReader(src, maxCustomArtworkBytes+1))
	closeErr := f.Close()
	switch {
	case copyErr != nil:
		_ = os.Remove(tmp) //nolint:errcheck // best-effort cleanup on an already-failing path
		span.RecordError(copyErr)
		return nil, fmt.Errorf("AddCustomArtwork: write: %w", copyErr)
	case closeErr != nil:
		_ = os.Remove(tmp) //nolint:errcheck // best-effort cleanup on an already-failing path
		span.RecordError(closeErr)
		return nil, fmt.Errorf("AddCustomArtwork: close: %w", closeErr)
	case written > maxCustomArtworkBytes:
		_ = os.Remove(tmp) //nolint:errcheck // best-effort cleanup on an already-failing path
		return nil, fmt.Errorf("AddCustomArtwork: image exceeds %d bytes", maxCustomArtworkBytes)
	case written == 0:
		_ = os.Remove(tmp) //nolint:errcheck // best-effort cleanup on an already-failing path
		return nil, errors.New("AddCustomArtwork: empty image")
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp) //nolint:errcheck // best-effort cleanup on an already-failing path
		span.RecordError(err)
		return nil, fmt.Errorf("AddCustomArtwork: publish: %w", err)
	}

	rec.LocalPath = &dest
	if err := s.artworkCmd.Create(ctx, rec); err != nil {
		// The row is the source of truth; an orphan file would be served by
		// nothing but would still occupy the volume.
		_ = os.Remove(dest) //nolint:errcheck // best-effort cleanup on an already-failing path
		span.RecordError(err)
		return nil, fmt.Errorf("AddCustomArtwork: persist: %w", err)
	}

	return rec, nil
}
