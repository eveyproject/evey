package postgres

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

// rowScanner abstracts sql.Row.Scan and sql.Rows.Scan so the row variant
// and the iterator variant can share the same column-mapping logic.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanMediaItemFields populates a flat MediaItem from the column set
// produced by mediaItemColumns + mediaItemFromJoins. Sub-aggregate
// columns come back NULL when the LEFT JOIN found no matching row, so
// every read is guarded with a sql.Null* helper or a *string pointer.
func scanMediaItemFields(src rowScanner, item *library.MediaItem) error {
	var (
		parentID, scanTitle                             *string
		filePath, fingerprint, containerFormat          *string
		fileSize, durationMs, bitrate                   *int64
		streamProbeAttempts, interleaveBackstepMs       sql.NullInt64
		modTimeRaw, latestChildAddedAt                  sql.NullTime
		premiereDate                                    sql.NullTime
		year                                            sql.NullInt64
		providerJSON, genresJSON, studiosJSON, tagsJSON []byte
		communityRating                                 sql.NullFloat64
		officialRating                                  sql.NullString
		certJSON, providerRatingsJSON, trailersJSON     []byte
	)

	if err := src.Scan(
		&item.ID, &item.LibraryID, &parentID, &item.Type, &item.Title, &scanTitle,
		&item.SortTitle, &item.OriginalTitle, &item.Overview, &item.Tagline, &item.IndexNumber,
		&item.AddedAt, &item.UpdatedAt, &latestChildAddedAt,
		&filePath, &fileSize, &fingerprint, &modTimeRaw, &durationMs, &bitrate, &containerFormat, &streamProbeAttempts, &interleaveBackstepMs,
		&premiereDate, &year, &providerJSON, &genresJSON, &studiosJSON, &tagsJSON,
		&communityRating, &officialRating, &certJSON, &providerRatingsJSON,
		&trailersJSON,
	); err != nil {
		return err
	}
	if parentID != nil {
		item.ParentID = *parentID
	}
	if latestChildAddedAt.Valid {
		t := latestChildAddedAt.Time
		item.LatestChildAddedAt = &t
	}
	if scanTitle != nil {
		item.ScanTitle = *scanTitle
	}

	item.File = buildMediaFile(filePath, fileSize, fingerprint, modTimeRaw, durationMs, bitrate, containerFormat, streamProbeAttempts)
	if item.File != nil && interleaveBackstepMs.Valid {
		item.File.InterleaveBackstepMs = interleaveBackstepMs.Int64
	}
	cat, err := buildMediaCatalog(premiereDate, year, providerJSON, genresJSON, studiosJSON, tagsJSON)
	if err != nil {
		return err
	}
	item.Catalog = cat
	item.Classification = buildMediaClassification(communityRating, officialRating, certJSON, providerRatingsJSON)

	// Trailers (1:N → JSONB array via subquery → flat slice).
	if len(trailersJSON) > 0 {
		if err := json.Unmarshal(trailersJSON, &item.Trailers); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "trailers"), slog.Any("err", err))
		}
	}

	return nil
}

// buildMediaFile returns the File sub-aggregate when the LEFT JOIN found
// a media_files row (filePath non-nil), else nil. Container types
// (series/season/album) leave item.File == nil.
func buildMediaFile(filePath *string, fileSize *int64, fingerprint *string, modTime sql.NullTime, durationMs, bitrate *int64, containerFormat *string, streamProbeAttempts sql.NullInt64) *library.MediaFile {
	if filePath == nil {
		return nil
	}
	f := &library.MediaFile{Path: *filePath}
	if streamProbeAttempts.Valid {
		f.StreamProbeAttempts = int(streamProbeAttempts.Int64)
	}
	if fileSize != nil {
		f.Size = *fileSize
	}
	if fingerprint != nil {
		f.Fingerprint = *fingerprint
	}
	if modTime.Valid {
		f.ModTime = modTime.Time
	}
	if durationMs != nil {
		// Negative duration_ms in postgres would be a corrupt row; clamp to
		// zero rather than fail the whole read so the rest of the item
		// stays usable. shared.NewDurationMs only errors on negative input.
		if d, err := shared.NewDurationMs(*durationMs); err == nil {
			f.Duration = d
		} else {
			slog.Warn("invalid duration_ms in media_files row", slog.String("path", *filePath), slog.Int64("ms", *durationMs), slog.Any("err", err))
		}
	}
	if bitrate != nil {
		f.Bitrate = *bitrate
	}
	if containerFormat != nil {
		f.ContainerFormat = *containerFormat
	}
	return f
}

// buildMediaCatalog returns the Catalog sub-aggregate when the LEFT JOIN
// found a media_catalog row OR any of the JSONB arrays came back
// non-empty, else nil. provider_ids + genres are treated as required
// (corrupt JSON fails the read instead of warning-and-dropping).
func buildMediaCatalog(premiereDate sql.NullTime, year sql.NullInt64, providerJSON, genresJSON, studiosJSON, tagsJSON []byte) (*library.MediaCatalog, error) {
	if !premiereDate.Valid && !year.Valid && len(providerJSON) == 0 && len(genresJSON) == 0 && len(studiosJSON) == 0 && len(tagsJSON) == 0 {
		return nil, nil
	}
	cat := &library.MediaCatalog{}
	if premiereDate.Valid {
		t := premiereDate.Time
		cat.PremiereDate = &t
	}
	if year.Valid {
		y := int(year.Int64)
		cat.Year = &y
	}
	if len(providerJSON) > 0 {
		if err := json.Unmarshal(providerJSON, &cat.ProviderIDs); err != nil {
			return nil, fmt.Errorf("unmarshal provider_ids: %w", err)
		}
	}
	if len(genresJSON) > 0 {
		if err := json.Unmarshal(genresJSON, &cat.Genres); err != nil {
			return nil, fmt.Errorf("unmarshal genres: %w", err)
		}
	}
	if len(studiosJSON) > 0 {
		if err := json.Unmarshal(studiosJSON, &cat.Studios); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "studios"), slog.Any("err", err))
		}
	}
	if len(tagsJSON) > 0 {
		if err := json.Unmarshal(tagsJSON, &cat.Tags); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "tags"), slog.Any("err", err))
		}
	}
	return cat, nil
}

// buildMediaClassification returns the Classification sub-aggregate when
// any rating-related column came back non-NULL, else nil.
func buildMediaClassification(communityRating sql.NullFloat64, officialRating sql.NullString, certJSON, providerRatingsJSON []byte) *library.MediaClassification {
	if !communityRating.Valid && !officialRating.Valid && len(certJSON) <= 2 && len(providerRatingsJSON) <= 2 {
		return nil
	}
	cl := &library.MediaClassification{}
	if communityRating.Valid {
		v := communityRating.Float64
		cl.CommunityRating = &v
	}
	if officialRating.Valid {
		cl.OfficialRating = officialRating.String
	}
	if len(certJSON) > 0 {
		if err := json.Unmarshal(certJSON, &cl.CertificationsByCountry); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "certifications_by_country"), slog.Any("err", err))
		}
	}
	if len(providerRatingsJSON) > 0 {
		if err := json.Unmarshal(providerRatingsJSON, &cl.ProviderRatings); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "provider_ratings"), slog.Any("err", err))
		}
	}
	return cl
}

func (r *MediaItemRepository) scanMediaItem(row *sql.Row) (*library.MediaItem, error) {
	item := &library.MediaItem{}
	err := scanMediaItemFields(row, item)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("MediaItemRepo.scanMediaItem: %w", err)
	}
	return item, nil
}

func (r *MediaItemRepository) scanMediaItemFromRows(rows *sql.Rows) (*library.MediaItem, error) {
	item := &library.MediaItem{}
	if err := scanMediaItemFields(rows, item); err != nil {
		return nil, fmt.Errorf("MediaItemRepo.scanMediaItemFromRows: %w", err)
	}
	return item, nil
}
