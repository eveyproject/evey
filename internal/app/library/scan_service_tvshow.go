package library

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/gravity-zero/evey/internal/domain/library"
)

func (s *ScanService) scanTVShowFiles(ctx context.Context, libraryID string, files []library.ScannedFile) (added, updated int, err error) {
	existing, err := s.mediaItems.ListByLibraryID(ctx, libraryID)
	if err != nil {
		return 0, 0, fmt.Errorf("preload containers: %w", err)
	}
	byFingerprint := make(map[string]*library.MediaItem, len(existing))
	type titleKey struct{ typ, title, parentID string }
	byTitleKey := make(map[titleKey]*library.MediaItem, len(existing))
	for _, item := range existing {
		if item.FileFingerprint != "" {
			byFingerprint[item.FileFingerprint] = item
		}
		byTitleKey[titleKey{item.Type, item.Title, item.ParentID}] = item
	}

	lookupOrCreate := func(item *library.MediaItem) (*library.MediaItem, bool, error) {
		if ex, ok := byFingerprint[item.FileFingerprint]; ok {
			return ex, false, nil
		}
		if ex, ok := byTitleKey[titleKey{item.Type, item.Title, item.ParentID}]; ok {
			if ex.FileFingerprint != item.FileFingerprint {
				ex.FileFingerprint = item.FileFingerprint
				if updateErr := s.mediaItems.Update(ctx, ex); updateErr != nil {
					slog.Warn("update media item fingerprint failed", slog.String("mediaItemID", ex.ID), slog.Any("err", updateErr))
				}
			}
			return ex, false, nil
		}
		if createErr := s.mediaItems.Create(ctx, item); createErr != nil {
			return nil, false, createErr
		}
		byFingerprint[item.FileFingerprint] = item
		byTitleKey[titleKey{item.Type, item.Title, item.ParentID}] = item
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
				ID:              uuid.New().String(),
				LibraryID:       libraryID,
				Type:            "series",
				Title:           showTitle,
				FileFingerprint: fp,
				AddedAt:         time.Now(),
				UpdatedAt:       time.Now(),
			})
			if createErr != nil {
				return 0, 0, fmt.Errorf("create series %q: %w", showTitle, createErr)
			}
			if created {
				evtSeriesAdded := library.MediaItemAdded{
					MediaItemID: seriesItem.ID,
					LibraryID:   libraryID,
					Title:       seriesItem.Title,
					Type:        seriesItem.Type,
					Genres:      seriesItem.Genres,
					OccurredAt:  time.Now(),
				}
				if err := s.events.Publish(ctx, evtSeriesAdded); err != nil {
					slog.Warn("publish event", slog.String("event", evtSeriesAdded.EventName()), slog.Any("err", err))
				}
				added++
			}
			seriesID = seriesItem.ID
			seriesIDs[showTitle] = seriesID
		}

		seasonKey := fmt.Sprintf("%s:%d", showTitle, seasonNum)
		seasonID, ok := seasonIDs[seasonKey]
		if !ok {
			fp := fmt.Sprintf("season:%s:%s:%d", libraryID, showTitle, seasonNum)
			n := seasonNum
			seasonItem, created, createErr := lookupOrCreate(&library.MediaItem{
				ID:              uuid.New().String(),
				LibraryID:       libraryID,
				ParentID:        seriesID,
				Type:            "season",
				Title:           fmt.Sprintf("Season %d", seasonNum),
				FileFingerprint: fp,
				IndexNumber:     &n,
				AddedAt:         time.Now(),
				UpdatedAt:       time.Now(),
			})
			if createErr != nil {
				return 0, 0, fmt.Errorf("create season %d for %q: %w", seasonNum, showTitle, createErr)
			}
			if created {
				added++
			}
			seasonID = seasonItem.ID
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

func (svc *ScanService) pruneEmptyContainers(ctx context.Context, libraryID string) (int, error) {
	all, err := svc.mediaItems.ListContainerInfo(ctx, libraryID)
	if err != nil {
		return 0, err
	}
	childCount := make(map[string]int)
	for _, item := range all {
		if item.ParentID != "" {
			childCount[item.ParentID]++
		}
	}
	pruned := 0
	for _, item := range all {
		if item.Type != string(library.Season) {
			continue
		}
		if childCount[item.ID] > 0 {
			continue
		}
		if dErr := svc.mediaItems.Delete(ctx, item.ID); dErr != nil {
			slog.Warn("delete empty season", slog.String("id", item.ID), slog.Any("err", dErr))
			continue
		}
		childCount[item.ParentID]--
		pruned++
	}
	for _, item := range all {
		if item.Type != string(library.Series) {
			continue
		}
		if childCount[item.ID] > 0 {
			continue
		}
		if dErr := svc.mediaItems.Delete(ctx, item.ID); dErr != nil {
			slog.Warn("delete empty series", slog.String("id", item.ID), slog.Any("err", dErr))
			continue
		}
		if svc.search != nil {
			if rmErr := svc.search.RemoveMediaItem(ctx, item.ID); rmErr != nil {
				slog.Warn("remove empty series from search index", slog.String("id", item.ID), slog.Any("err", rmErr))
			}
		}
		pruned++
	}
	return pruned, nil
}
