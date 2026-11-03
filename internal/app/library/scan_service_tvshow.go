package library

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
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

// refileStaleParse moves an item read by an older parser to where the current
// parser files its path: same row, same id, same watch history, new type,
// parent and episode number. Only a parse that names an episode refiles, and
// only rows below library.ParserVersion are touched, so an item the parser
// has already placed is left alone. The display title is reset only when the
// row changes type: it was derived from a name the old parser misread.
//
// A row that changes type also loses what enrichment matched for its old
// type: a provider id from the movie space, the movie's overview, cast and
// artwork describe another work. They are dropped and the item is announced
// as added, so the enricher matches it again as an episode. The id, and with
// it the progress, favourites and playlists, stays.
func (s *ScanService) refileStaleParse(ctx context.Context, existing *library.MediaItem, seasonID string, f library.ScannedFile, showTitle string) {
	if existing == nil || existing.File == nil || existing.File.ParserVersion >= library.ParserVersion || f.Episode == nil {
		return
	}
	// A row already typed episode with the right number only needs a new
	// parent, which updateMediaFile writes (and reports) for any moved file:
	// refiling it here too wrote the row twice and hid that update's errors.
	sameIndex := existing.IndexNumber != nil && *existing.IndexNumber == *f.Episode
	if existing.Type == library.Episode && sameIndex {
		return
	}
	// Refiled on a copy: a failed write leaves the caller's row as it was,
	// so the regular update still sees, writes and reports the move.
	prior := existing
	cp := *existing
	fileCopy := *existing.File
	cp.File = &fileCopy
	if existing.Catalog != nil {
		catCopy := *existing.Catalog
		cp.Catalog = &catCopy
	}
	existing = &cp
	retyped := existing.Type != library.Episode
	if retyped {
		existing.Type = library.Episode
		existing.Title = f.Title
		existing.Overview, existing.Tagline, existing.OriginalTitle = "", "", ""
		if existing.Catalog != nil {
			existing.Catalog.ProviderIDs = nil
		}
	}
	existing.ScanTitle = f.Title
	existing.ParentID = seasonID
	existing.IndexNumber = new(int)
	*existing.IndexNumber = *f.Episode
	existing.File.ParserVersion = library.ParserVersion
	existing.UpdatedAt = time.Now()
	if err := s.mediaItems.Update(ctx, existing); err != nil {
		slog.WarnContext(ctx, "scan: refile item under its season failed",
			slog.String("media_id", string(existing.ID)), slog.Any("err", err))
		return
	}
	*prior = cp
	existing = prior
	s.indexItem(ctx, existing)
	if !retyped {
		return
	}
	if s.mediaPersons != nil {
		if err := s.mediaPersons.DeleteByMediaItemID(ctx, existing.ID); err != nil {
			slog.WarnContext(ctx, "scan: drop the cast of a refiled item", slog.String("media_id", string(existing.ID)), slog.Any("err", err))
		}
	}
	if s.artworkPurger != nil {
		if err := s.artworkPurger.DeleteByMediaItemID(ctx, string(existing.ID)); err != nil {
			slog.WarnContext(ctx, "scan: drop the artwork of a refiled item", slog.String("media_id", string(existing.ID)), slog.Any("err", err))
		}
	}
	s.publish(ctx, library.MediaItemAdded{
		MediaItemID: existing.ID,
		LibraryID:   existing.LibraryID,
		Title:       existing.Title,
		SeriesTitle: showTitle,
		Type:        existing.Type,
		Genres:      catalogGenres(existing),
		OccurredAt:  time.Now(),
	})
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
					slog.WarnContext(ctx, "update media item fingerprint failed", slog.String("media_id", string(ex.ID)), slog.Any("err", updateErr))
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
			seriesOpts := []library.MediaItemOption{
				library.WithMediaItemID(shared.MediaItemID(uuid.New().String())),
				library.WithMediaItemFile(&library.MediaFile{Fingerprint: fp}),
			}
			// The year the user wrote on the show folder ("Sherlock Holmes
			// (1954)/") is the one hint that tells remakes apart; without it
			// the provider search picked the 1984 series for a 1954 one.
			if f.Year != nil {
				seriesOpts = append(seriesOpts, library.WithMediaItemYear(*f.Year))
			}
			newSeries, buildErr := library.NewMediaItem(shared.LibraryID(libraryID), library.Series, showTitle, seriesOpts...)
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
		s.refileStaleParse(ctx, byPath[f.Path], seasonID, episodeFile, showTitle)
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
