package library

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

func (s *ScanService) scanTVShowFiles(ctx context.Context, libraryID string, files []library.ScannedFile) (added, updated int, err error) {
	existing, err := s.mediaItems.ListByLibraryID(ctx, shared.LibraryID(libraryID))
	if err != nil {
		return 0, 0, fmt.Errorf("preload containers: %w", err)
	}
	byFingerprint := make(map[string]*library.MediaItem, len(existing))
	// Dedup by scan_title (stable FS-derived) instead of Title. Title gets
	// overwritten by metadata enrichment after creation, so a subsequent
	// scan that looks up by the original filesystem title would never find
	// the already-enriched row and would create a duplicate every pass.
	type titleKey struct {
		typ                 library.MediaItemType
		scanTitle, parentID string
	}
	byTitleKey := make(map[titleKey]*library.MediaItem, len(existing))
	itemFP := func(m *library.MediaItem) string {
		if m == nil || m.File == nil {
			return ""
		}
		return m.File.Fingerprint
	}
	scanKey := func(m *library.MediaItem) string {
		if m.ScanTitle != "" {
			return m.ScanTitle
		}
		return m.Title
	}
	for _, item := range existing {
		if fp := itemFP(item); fp != "" {
			byFingerprint[fp] = item
		}
		byTitleKey[titleKey{item.Type, scanKey(item), item.ParentID}] = item
	}

	lookupOrCreate := func(item *library.MediaItem) (*library.MediaItem, bool, error) {
		fp := itemFP(item)
		if ex, ok := byFingerprint[fp]; ok {
			return ex, false, nil
		}
		if ex, ok := byTitleKey[titleKey{item.Type, scanKey(item), item.ParentID}]; ok {
			if itemFP(ex) != fp {
				if ex.File == nil {
					ex.File = &library.MediaFile{}
				}
				ex.File.Fingerprint = fp
				if updateErr := s.mediaItems.Update(ctx, ex); updateErr != nil {
					slog.Warn("update media item fingerprint failed", slog.String("mediaItemID", string(ex.ID)), slog.Any("err", updateErr))
				}
			}
			return ex, false, nil
		}
		if createErr := s.mediaItems.Create(ctx, item); createErr != nil {
			return nil, false, createErr
		}
		byFingerprint[fp] = item
		byTitleKey[titleKey{item.Type, scanKey(item), item.ParentID}] = item
		return item, true, nil
	}

	seriesIDs := map[string]string{}
	seasonIDs := map[string]string{}

	for _, f := range files {
		showTitle := f.Title
		seasonNum := 0
		if f.Season != nil {
			seasonNum = *f.Season
		}

		seriesID, ok := seriesIDs[showTitle]
		if !ok {
			fp := fmt.Sprintf("series:%s:%s", libraryID, showTitle)
			seriesItem, created, createErr := lookupOrCreate(&library.MediaItem{
				ID:        shared.MediaItemID(uuid.New().String()),
				LibraryID: shared.LibraryID(libraryID),
				Type:      library.Series,
				Title:     showTitle,
				ScanTitle: showTitle,
				File:      &library.MediaFile{Fingerprint: fp},
				AddedAt:   time.Now(),
				UpdatedAt: time.Now(),
			})
			if createErr != nil {
				return 0, 0, fmt.Errorf("create series %q: %w", showTitle, createErr)
			}
			if created {
				s.publish(ctx, library.MediaItemAdded{
					MediaItemID: seriesItem.ID,
					LibraryID:   shared.LibraryID(libraryID),
					Title:       seriesItem.Title,
					Type:        string(seriesItem.Type),
					Genres:      catalogGenres(seriesItem),
					OccurredAt:  time.Now(),
				})
				added++
			}
			seriesID = string(seriesItem.ID)
			seriesIDs[showTitle] = seriesID
		}

		seasonKey := fmt.Sprintf("%s:%d", showTitle, seasonNum)
		seasonID, ok := seasonIDs[seasonKey]
		if !ok {
			fp := fmt.Sprintf("season:%s:%s:%d", libraryID, showTitle, seasonNum)
			n := seasonNum
			// ScanTitle stays "Season N" (the FS-derived label). Display
			// title gets localized/enriched later; the dedup key in
			// lookupOrCreate uses ScanTitle so a re-scan never creates a
			// new row just because enrichment renamed "Season 1" →
			// "Saison 1".
			scanSeasonTitle := fmt.Sprintf("Season %d", seasonNum)
			seasonItem, created, createErr := lookupOrCreate(&library.MediaItem{
				ID:          shared.MediaItemID(uuid.New().String()),
				LibraryID:   shared.LibraryID(libraryID),
				ParentID:    seriesID,
				Type:        library.Season,
				Title:       scanSeasonTitle,
				ScanTitle:   scanSeasonTitle,
				File:        &library.MediaFile{Fingerprint: fp},
				IndexNumber: &n,
				AddedAt:     time.Now(),
				UpdatedAt:   time.Now(),
			})
			if createErr != nil {
				return 0, 0, fmt.Errorf("create season %d for %q: %w", seasonNum, showTitle, createErr)
			}
			if created {
				added++
			}
			seasonID = string(seasonItem.ID)
			seasonIDs[seasonKey] = seasonID
		}

		episodeFile := f
		if f.Episode != nil {
			se := 0
			if f.Season != nil {
				se = *f.Season
			}
			episodeFile.Title = fmt.Sprintf("S%02dE%02d", se, *f.Episode)
		}
		a, u, episodeErr := s.upsertMediaFile(ctx, libraryID, seasonID, episodeFile, showTitle)
		if episodeErr != nil {
			return 0, 0, episodeErr
		}
		added += a
		updated += u
	}
	return added, updated, nil
}

