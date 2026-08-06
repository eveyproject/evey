package library

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/google/uuid"
)

// normShowKey is the case-insensitive grouping key for a show/season title.
// Show titles are parsed from the FILENAME (not the folder), and renamers are
// inconsistent about casing ("The Boys S02E01" vs "The boys S05E08"), so a
// case-sensitive key splits the same show into two SERIES rows whose seasons
// then live under different parents. Lower-casing + trimming collapses every
// casing of one show onto a single series. The display Title / ScanTitle keep
// their original case (and get overwritten by metadata enrichment anyway).
func normShowKey(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func (s *ScanService) scanTVShowFiles(ctx context.Context, libraryID string, files []library.ScannedFile) (added, updated int, err error) {
	existing, err := s.mediaItems.ListByLibraryID(ctx, shared.LibraryID(libraryID))
	if err != nil {
		return 0, 0, fmt.Errorf("preload containers: %w", err)
	}
	// Path index over the library we just loaded, so the per-episode upsert
	// below resolves in memory instead of issuing one SELECT per episode on
	// every scan pass.
	byPath := make(map[string]*library.MediaItem, len(existing))
	for _, m := range existing {
		if m != nil && m.File != nil && m.File.Path != "" {
			byPath[m.File.Path] = m
		}
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
			return normShowKey(m.ScanTitle)
		}
		return normShowKey(m.Title)
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
					slog.WarnContext(ctx, "update media item fingerprint failed", slog.String("mediaItemID", string(ex.ID)), slog.Any("err", updateErr))
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
		// showKey groups all casings of one show. Used for the per-scan maps
		// and the synthetic fingerprints so they stay stable regardless of how
		// each episode file happens to be cased; showTitle keeps its case for
		// the display Title.
		showKey := normShowKey(showTitle)
		seasonNum := 0
		if f.Season != nil {
			seasonNum = *f.Season
		}

		seriesID, ok := seriesIDs[showKey]
		if !ok {
			fp := fmt.Sprintf("series:%s:%s", libraryID, showKey)
			// Build through the canonical constructor so a blank/whitespace-only
			// title parsed from a malformed filename is rejected (ErrEmptyTitle)
			// instead of silently persisting as an unnamed series row. Mirrors
			// the movie/episode path in createMediaFile (scan_service.go).
			newSeries, buildErr := library.NewMediaItem(shared.LibraryID(libraryID), library.Series, showTitle,
				library.WithMediaItemID(shared.MediaItemID(uuid.New().String())),
				library.WithMediaItemFile(&library.MediaFile{Fingerprint: fp}),
			)
			if buildErr != nil {
				slog.WarnContext(ctx, "scan: skipping series that fails domain invariants",
					slog.String("path", f.Path), slog.Any("err", buildErr))
				continue
			}
			seriesItem, created, createErr := lookupOrCreate(newSeries)
			if createErr != nil {
				return 0, 0, fmt.Errorf("create series %q: %w", showTitle, createErr)
			}
			if created {
				s.publish(ctx, library.MediaItemAdded{
					MediaItemID: seriesItem.ID,
					LibraryID:   shared.LibraryID(libraryID),
					Title:       seriesItem.Title,
					Type:        seriesItem.Type,
					Genres:      catalogGenres(seriesItem),
					OccurredAt:  time.Now(),
				})
				added++
			}
			seriesID = string(seriesItem.ID)
			seriesIDs[showKey] = seriesID
		}

		seasonKey := fmt.Sprintf("%s:%d", showKey, seasonNum)
		seasonID, ok := seasonIDs[seasonKey]
		if !ok {
			fp := fmt.Sprintf("season:%s:%s:%d", libraryID, showKey, seasonNum)
			n := seasonNum
			// ScanTitle stays "Season N" (the FS-derived label). Display
			// title gets localized/enriched later; the dedup key in
			// lookupOrCreate uses ScanTitle so a re-scan never creates a
			// new row just because enrichment renamed "Season 1" →
			// "Saison 1".
			scanSeasonTitle := fmt.Sprintf("Season %d", seasonNum)
			newSeason, buildErr := library.NewMediaItem(shared.LibraryID(libraryID), library.Season, scanSeasonTitle,
				library.WithMediaItemID(shared.MediaItemID(uuid.New().String())),
				library.WithMediaItemParent(seriesID),
				library.WithMediaItemFile(&library.MediaFile{Fingerprint: fp}),
				library.WithMediaItemIndexNumber(n),
			)
			if buildErr != nil {
				slog.WarnContext(ctx, "scan: skipping season that fails domain invariants",
					slog.String("path", f.Path), slog.Any("err", buildErr))
				continue
			}
			seasonItem, created, createErr := lookupOrCreate(newSeason)
			if createErr != nil {
				return 0, 0, fmt.Errorf("create season %d for %q: %w", seasonNum, showTitle, createErr)
			}
			if created {
				// Publish like series/episodes do: the enricher routes a Season
				// MediaItemAdded to autoEnrichSeason (enricher_auto.go). Without
				// this a newly scanned season never auto-enriches - it stays with
				// no season poster/overview/air-date until the daily re-enrich or
				// a manual "Refresh metadata".
				s.publish(ctx, library.MediaItemAdded{
					MediaItemID: seasonItem.ID,
					LibraryID:   shared.LibraryID(libraryID),
					Title:       seasonItem.Title,
					SeriesTitle: showTitle, // groups seasons with their series' batch
					// (one "new series" notification instead of one per season) AND
					// names the series in a standalone "new season" notification.
					Type:       seasonItem.Type,
					Genres:     catalogGenres(seasonItem),
					OccurredAt: time.Now(),
				})
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
		a, u, episodeErr := s.upsertMediaFileWithIndex(ctx, libraryID, seasonID, episodeFile, byPath, showTitle)
		if episodeErr != nil {
			// Skip this episode and keep scanning the rest - a single transient
			// failure must not abort the whole library scan (the movie path
			// already logs-and-continues; this kept aborting it).
			slog.WarnContext(ctx, "scan: skipping episode after upsert error",
				slog.String("path", episodeFile.Path), slog.Any("err", episodeErr))
			continue
		}
		added += a
		updated += u
	}
	return added, updated, nil
}
