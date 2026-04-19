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
		modTimeRaw                                      sql.NullTime
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
		&item.AddedAt, &item.UpdatedAt,
		&filePath, &fileSize, &fingerprint, &modTimeRaw, &durationMs, &bitrate, &containerFormat,
		&premiereDate, &year, &providerJSON, &genresJSON, &studiosJSON, &tagsJSON,
		&communityRating, &officialRating, &certJSON, &providerRatingsJSON,
		&trailersJSON,
	); err != nil {
		return err
	}
	if parentID != nil {
		item.ParentID = *parentID
	}
	if scanTitle != nil {
		item.ScanTitle = *scanTitle
	}

	// File sub-aggregate. Allocated only when the LEFT JOIN found a
	// media_files row (filePath non-nil). Container types (series/season/
	// album) leave item.File == nil.
	if filePath != nil {
		f := &library.MediaFile{Path: *filePath}
		if fileSize != nil {
			f.Size = *fileSize
		}
		if fingerprint != nil {
			f.Fingerprint = *fingerprint
		}
		if modTimeRaw.Valid {
			f.ModTime = modTimeRaw.Time
		}
		if durationMs != nil {
			f.DurationMs = *durationMs
		}
		if bitrate != nil {
			f.Bitrate = *bitrate
		}
		if containerFormat != nil {
			f.ContainerFormat = *containerFormat
		}
		item.File = f
	}

	// Catalog sub-aggregate. Allocated only when the LEFT JOIN found a
	// media_catalog row OR any of the JSONB arrays came back non-empty.
	if premiereDate.Valid || year.Valid || len(providerJSON) > 0 || len(genresJSON) > 0 || len(studiosJSON) > 0 || len(tagsJSON) > 0 {
		cat := &library.MediaCatalog{}
		if premiereDate.Valid {
			t := premiereDate.Time
			cat.PremiereDate = &t
		}
		if year.Valid {
			y := int(year.Int64)
			cat.Year = &y
		}
		// Required: provider_ids and genres drive joins/filters; corrupt JSON
		// here is a real DB integrity error, not a soft warning.
		if len(providerJSON) > 0 {
			if err := json.Unmarshal(providerJSON, &cat.ProviderIDs); err != nil {
				return fmt.Errorf("unmarshal provider_ids: %w", err)
			}
		}
		if len(genresJSON) > 0 {
			if err := json.Unmarshal(genresJSON, &cat.Genres); err != nil {
				return fmt.Errorf("unmarshal genres: %w", err)
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
		item.Catalog = cat
	}

	// Classification sub-aggregate. Allocated only when at least one
	// rating-related column came back non-NULL.
	if communityRating.Valid || officialRating.Valid || len(certJSON) > 2 || len(providerRatingsJSON) > 2 {
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
		item.Classification = cl
	}

	// Trailers (1:N → JSONB array via subquery → flat slice).
	if len(trailersJSON) > 0 {
		if err := json.Unmarshal(trailersJSON, &item.Trailers); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "trailers"), slog.Any("err", err))
		}
	}

	return nil
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
