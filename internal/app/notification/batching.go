package notification

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	libevent "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/notification"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
	"github.com/google/uuid"
)

// batching.go holds the "new media added" pipeline: the stateful batch buffer
// (dedupe + per-series timers) and the fan-out that turns a batched event into
// per-profile notifications. It is the only responsibility that owns mutable
// state on CommandService (s.mu / s.dedupe / s.batches).

type batchEntry struct {
	seriesEvent *libevent.MediaItemAdded
	seasons     []libevent.MediaItemAdded
	episodes    []libevent.MediaItemAdded
	timer       *time.Timer
	createdAt   time.Time
}

const (
	defaultBatchDelay  = 5 * time.Second
	maxBatchEntries    = 1000
	episodeGroupWindow = 24 * time.Hour
)

// FlushBatches forces all pending batches to flush immediately. Useful for tests.
func (s *CommandService) FlushBatches() {
	s.mu.Lock()
	keys := make([]string, 0, len(s.batches))
	for k, b := range s.batches {
		if b.timer != nil {
			b.timer.Stop()
		}
		keys = append(keys, k)
	}
	s.mu.Unlock()

	for _, k := range keys {
		s.flushBatch(k)
	}
}

func (s *CommandService) handleMediaItemAdded(_ context.Context, ev shared.Event) {
	e, ok := ev.(libevent.MediaItemAdded)
	if !ok {
		return
	}

	// Skip image types entirely
	if e.Type == libevent.PhotoItem {
		return
	}

	dedupeKey := fmt.Sprintf("%s:%s", e.Type, e.MediaItemID)
	s.mu.Lock()
	if last, exists := s.dedupe[dedupeKey]; exists && time.Since(last) < dedupeWindow {
		s.mu.Unlock()
		return
	}
	s.dedupe[dedupeKey] = time.Now()

	// Series-related types with a series title get batched
	if isBatchable(e) {
		s.bufferSeriesEvent(e)
		s.mu.Unlock()
		return
	}

	// Albums get batched by album title (tracks amalgamated). Untagged tracks
	// (no SeriesTitle) are batched too: batchKey falls back to the track title,
	// and flushBatch's Track branch names the album by that title. Without this
	// they fell through to the direct emit below, where typeAndTitle has no
	// "track" case and drops them (an untagged audio file notified nothing).
	if e.Type == libevent.Album || e.Type == libevent.Track {
		s.bufferSeriesEvent(e)
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	s.emitNotifications(e.Type, e.Title, e.SeriesTitle, string(e.MediaItemID), "", e.Genres, nil, labelParts{subject: subjectOf(e.Title, e.SeriesTitle)})
}

func isBatchable(e libevent.MediaItemAdded) bool {
	switch e.Type {
	case libevent.Series, libevent.Season:
		return true
	case libevent.Episode:
		return e.SeriesTitle != ""
	}
	return false
}

func (s *CommandService) batchKey(e libevent.MediaItemAdded) string {
	// Albums use their own title as key, series use SeriesTitle
	if e.Type == libevent.Album || e.Type == libevent.Track {
		key := e.SeriesTitle
		if key == "" {
			key = e.Title
		}
		return fmt.Sprintf("album:%s:%s", e.LibraryID, key)
	}
	key := e.SeriesTitle
	if key == "" {
		key = e.Title
	}
	return fmt.Sprintf("series:%s:%s", e.LibraryID, key)
}

// bufferSeriesEvent collects related events and flushes after batchDelay.
// Caller must hold s.mu.
func (s *CommandService) bufferSeriesEvent(e libevent.MediaItemAdded) {
	key := s.batchKey(e)
	batch, exists := s.batches[key]
	if !exists {
		if len(s.batches) >= maxBatchEntries {
			var oldestKey string
			var oldestTime time.Time
			for k, b := range s.batches {
				created := b.createdAt
				if oldestKey == "" || created.Before(oldestTime) {
					oldestKey = k
					oldestTime = created
				}
			}
			// Flush the evicted batch instead of discarding it: eviction is a
			// capacity decision, the user still deserves the notification the
			// batch was accumulating. Pop the entry NOW, under the held lock -
			// a deferred pop made the cap advisory: until the detached flush
			// re-acquired s.mu, every new key re-elected the SAME oldest entry
			// and the map grew past maxBatchEntries under a burst.
			evicted := s.batches[oldestKey]
			delete(s.batches, oldestKey)
			if evicted != nil && evicted.timer != nil {
				evicted.timer.Stop()
			}
			go func(b *batchEntry) {
				defer func() {
					if r := recover(); r != nil {
						slog.Error("panic recovered in evicted notification batch flush",
							slog.Any("recover", r), slog.String("stack", string(debug.Stack())))
					}
				}()
				s.flushEntry(b)
			}(evicted)
			slog.Warn("notification batch map full, flushing oldest entry early", slog.String("key", oldestKey))
		}
		batch = &batchEntry{createdAt: time.Now()}
		s.batches[key] = batch
	}

	switch e.Type {
	case libevent.Series, libevent.Album:
		batch.seriesEvent = &e
	case libevent.Season:
		batch.seasons = append(batch.seasons, e)
	case libevent.Episode, libevent.Track:
		batch.episodes = append(batch.episodes, e)
	}

	// Reset timer on each new event
	if batch.timer != nil {
		batch.timer.Stop()
	}
	batch.timer = time.AfterFunc(s.batchDelay, func() {
		s.flushBatch(key)
	})
}

func (s *CommandService) flushBatch(key string) {
	s.mu.Lock()
	batch, exists := s.batches[key]
	if !exists {
		s.mu.Unlock()
		return
	}
	delete(s.batches, key)
	s.mu.Unlock()

	s.flushEntry(batch)
}

// flushEntry emits the notifications for an already-popped batch entry.
// Callers that still hold the entry in s.batches go through flushBatch.
func (s *CommandService) flushEntry(batch *batchEntry) {
	if batch == nil {
		return
	}

	seasonCount := len(batch.seasons)
	episodeCount := len(batch.episodes)

	// Full series (series event present)
	if batch.seriesEvent != nil {
		if batch.seriesEvent.Type == libevent.Album {
			title, push := batchedAlbumTitle(batch.seriesEvent.Title, episodeCount)
			s.emitNotifications(libevent.Album, title, "", string(batch.seriesEvent.MediaItemID), "", batch.seriesEvent.Genres, &push, labelParts{subject: batch.seriesEvent.Title, episodeCount: episodeCount})
			return
		}
		seriesKey := notification.NewSeriesKey(string(batch.seriesEvent.LibraryID), batch.seriesEvent.Title)
		title, push := batchedSeriesTitle(batch.seriesEvent.Title, seasonCount, episodeCount)
		s.emitNotifications(libevent.Series, title, "", string(batch.seriesEvent.MediaItemID), seriesKey, batch.seriesEvent.Genres, &push, labelParts{subject: batch.seriesEvent.Title, seasonCount: seasonCount, episodeCount: episodeCount})
		return
	}

	// Seasons without series event (e.g. new season added to existing series)
	if seasonCount > 0 && episodeCount > 0 {
		season := batch.seasons[0]
		seriesTitle := season.SeriesTitle
		if seriesTitle == "" {
			seriesTitle = season.Title
		}
		seriesKey := notification.NewSeriesKey(string(season.LibraryID), season.SeriesTitle)
		title, push := batchedSeasonTitle(seriesTitle, season.Title, episodeCount)
		s.emitNotifications(libevent.Season, title, "", string(season.MediaItemID), seriesKey, season.Genres, &push, labelParts{subject: seriesTitle, seasonCount: seasonCount, episodeCount: episodeCount})
		return
	}

	// Season(s) alone - group into ONE notification that names the series,
	// instead of one bare "Season N" per season.
	if seasonCount > 0 {
		season := batch.seasons[0]
		seriesKey := notification.NewSeriesKey(string(season.LibraryID), season.SeriesTitle)
		title, push := batchedSeasonsTitle(season.SeriesTitle, season.Title, seasonCount)
		s.emitNotifications(libevent.Season, title, "", string(season.MediaItemID), seriesKey, season.Genres, &push, labelParts{subject: season.SeriesTitle, seasonCount: seasonCount})
		return
	}

	// Tracks that reached the batch without an album event (added to an
	// existing album, or the album event landed in an earlier window): name the
	// album, never "New Episode". Mirrors the album branch above so the client
	// renders "Album (N tracks)" and the profile's music level governs it.
	if episodeCount > 0 && batch.episodes[0].Type == libevent.Track {
		first := batch.episodes[0]
		albumTitle := first.SeriesTitle
		if albumTitle == "" {
			albumTitle = first.Title
		}
		title, push := batchedAlbumTitle(albumTitle, episodeCount)
		s.emitNotifications(libevent.Album, title, "", string(first.MediaItemID), "", first.Genres, &push, labelParts{subject: albumTitle, episodeCount: episodeCount})
		return
	}

	// Just episodes
	if episodeCount == 1 {
		ep := batch.episodes[0]
		seriesKey := notification.NewSeriesKey(string(ep.LibraryID), ep.SeriesTitle)
		s.emitNotifications(ep.Type, ep.Title, ep.SeriesTitle, string(ep.MediaItemID), seriesKey, ep.Genres, nil, labelParts{subject: subjectOf(ep.Title, ep.SeriesTitle), episodeCount: 1})
		return
	}

	if episodeCount > 1 {
		first := batch.episodes[0]
		seriesKey := notification.NewSeriesKey(string(first.LibraryID), first.SeriesTitle)
		title, push := batchedEpisodesTitle(first.SeriesTitle, episodeCount)
		s.emitNotifications(libevent.Episode, title, "", string(first.MediaItemID), seriesKey, first.Genres, &push, labelParts{subject: first.SeriesTitle, episodeCount: episodeCount})
	}
}

// subjectOf picks the name the COUNTS apply to: the series for an episode, the
// item itself otherwise. Using the episode title here regrouped "The Wire" into
// "S01E07 (2 episodes)" - the subject must survive the regrouping, and only the
// series does.
func subjectOf(title, seriesTitle string) string {
	if seriesTitle != "" {
		return seriesTitle
	}
	return title
}

// labelParts carries what a CLIENT needs to build the sentence in its own
// language: the bare subject ("Some Series") and the counts. The pre-rendered
// English Title stays alongside for rows and clients that predate this - see
// the 000151 migration for why nothing is backfilled.
type labelParts struct {
	subject      string
	seasonCount  int
	episodeCount int
}

func (s *CommandService) emitNotifications(mediaType libevent.MediaItemType, title, seriesTitle, mediaItemID string, seriesKey notification.SeriesKey, genres []string, pushOverride *pushSpec, parts labelParts) {
	parent := s.shutdownCtx
	if parent == nil {
		slog.Warn("notification: emitNotifications called before Start(); using background context")
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()

	notifType, notifTitle, push := typeAndTitle(mediaType, title, seriesTitle)
	if notifType == "" {
		return
	}
	if pushOverride != nil {
		push = *pushOverride
	}

	profiles, err := s.profiles.GetAll(ctx)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			slog.WarnContext(ctx, "notification: list profiles failed", slog.Any("err", err))
		}
		return
	}

	profileIDs := make([]shared.ProfileID, len(profiles))
	for i, p := range profiles {
		profileIDs[i] = p.ID
	}
	allSettings, err := s.settings.GetByProfiles(ctx, profileIDs)
	if err != nil {
		slog.WarnContext(ctx, "notification: get settings batch failed", slog.Any("err", err))
		return
	}

	favoritesByProfile, err := s.fetchFavoriteGenres(ctx, profiles, allSettings, notifType, genres)
	if err != nil {
		// A genre-analytics failure must NOT suppress notifications for every
		// profile. LevelAll profiles don't use genre relevance, so continue with
		// no favorites: shouldNotify still notifies them, and only Relevant/
		// HighlyRelevant profiles fail-closed to "no genre match" for this one
		// notification (err already logged inside fetchFavoriteGenres).
		favoritesByProfile = nil
	}

	initialEpCount := parts.episodeCount

	expires := time.Now().AddDate(0, 0, defaultRetentionDays)
	since := time.Now().Add(-episodeGroupWindow)

	// Preload dedup candidates for every profile in ONE query instead of one
	// query per profile inside the loop below (was the N+1: profiles x series
	// on a library scan). A preload failure fails open: recentByProfile stays
	// empty and preloadFailed makes shouldSkipEpisodeNotif never skip, so this
	// batch behaves exactly like the per-profile query erroring used to.
	var recentByProfile map[shared.ProfileID][]*notification.Notification
	preloadFailed := false
	if notifType == notification.TypeNewEpisode && seriesKey != "" {
		dedupTypes := []notification.Type{notification.TypeNewSeries, notification.TypeNewSeason, notification.TypeNewEpisode}
		var preloadErr error
		recentByProfile, preloadErr = s.dedupReads.FindRecentByProfilesTypeAndSeries(ctx, profileIDs, dedupTypes, seriesKey, since)
		if preloadErr != nil {
			slog.WarnContext(ctx, "notification: preload recent series/season/episode notifications failed", slog.Any("err", preloadErr))
			preloadFailed = true
		}
	}

	fanoutID := uuid.New().String()
	for _, p := range profiles {
		cfg := allSettings[p.ID]
		if cfg == nil {
			continue
		}
		lvl := levelForType(cfg, notifType)
		if !shouldNotify(lvl, genres, favoritesByProfile[p.ID]) {
			continue
		}

		// For episode notifications with a series key, check for recent series/season
		// notifications (skip) or recent episode notification (group/update).
		if notifType == notification.TypeNewEpisode && seriesKey != "" {
			if s.shouldSkipEpisodeNotif(ctx, p.ID, recentByProfile[p.ID], preloadFailed, p.Locale, initialEpCount) {
				continue
			}
		}

		opts := []notification.NotificationOption{
			notification.WithNotificationMediaItem(shared.MediaItemID(mediaItemID)),
			notification.WithNotificationLevel(lvl),
			notification.WithNotificationExpiresAt(&expires),
		}
		if seriesKey != "" {
			opts = append(opts, notification.WithSeriesKey(seriesKey))
		}
		// The running-total logic that must survive regrouping lives in
		// shouldSkipEpisodeNotif above (it updates the existing row's count);
		// here the counts this batch carried are the ones to persist.
		opts = append(opts, notification.WithLabelParts(parts.subject, parts.seasonCount, parts.episodeCount))
		n, err := notification.NewNotification(uuid.New().String(), p.ID, notifType, notifTitle, opts...)
		if err != nil {
			slog.WarnContext(ctx, "notification build failed", slog.String("profileID", string(p.ID)), slog.Any("err", err))
			continue
		}
		s.persistAndDispatch(ctx, n, push, p.Locale, fanoutID)
	}
}

// shouldSkipEpisodeNotif returns true if the episode notification should be
// suppressed (series/season notif already sent recently) or updated in-place
// (an existing grouped episode notif is within the grouping window).
// When it updates an existing notification, it also returns true so the caller
// skips creating a new one.
// recentNotifs is this profile's slice of the batch preloaded once per series
// flush by the caller (see emitNotifications); preloadFailed signals that the
// preload query itself errored, in which case we fail open exactly like the
// old per-profile query did on error: never skip, always create.
// initialCount is the number of episodes this batch represents (1 for a single
// episode, N for a pre-batched group). It is used to correctly compute the
// accumulated count when merging into an existing notification.
func (s *CommandService) shouldSkipEpisodeNotif(ctx context.Context, profileID shared.ProfileID, recentNotifs []*notification.Notification, preloadFailed bool, locale string, initialCount int) bool {
	if preloadFailed {
		return false // fail open: create the notification
	}

	// Case A/B: series or season notification already sent for this series → skip.
	for _, n := range recentNotifs {
		if n.Type == notification.TypeNewSeries || n.Type == notification.TypeNewSeason {
			return true
		}
	}

	// Case C: existing grouped episode notification within window → update it.
	var existingEps []*notification.Notification
	for _, n := range recentNotifs {
		if n.Type == notification.TypeNewEpisode {
			existingEps = append(existingEps, n)
		}
	}
	if len(existingEps) == 0 {
		return false
	}

	// Update the most recent one.
	existing := existingEps[0]
	addCount := max(initialCount, 1)
	// treat the original notification as representing at least 1 episode
	prevCount := max(existing.EpisodeCount, 1)
	newCount := prevCount + addCount

	// Prefer the stored Subject: it is the series name as data. Parsing it back
	// out of the rendered English title is what this whole change replaces, and
	// it stays only for rows written before Subject existed.
	seriesName := existing.Subject
	if seriesName == "" {
		seriesName = extractSeriesName(existing.Title)
	}
	newTitle, newPush := batchedEpisodesTitle(seriesName, newCount)
	existing.Title = "new_episode:" + newTitle
	existing.Subject = seriesName
	existing.EpisodeCount = newCount
	if updateErr := s.repo.Update(ctx, existing); updateErr != nil {
		slog.WarnContext(ctx, "notification: update grouped episode failed", slog.Any("err", updateErr))
		return false // fail open
	}
	// Re-send push for the updated notification. No notification.created here:
	// the e-mail went out with the first row, a season drop is not N e-mails.
	s.sendPush(ctx, profileID, newPush.resolveTitle(s.translator, locale), existing.Message)
	return true
}

func (s *CommandService) fetchFavoriteGenres(
	ctx context.Context,
	profiles []*user.Profile,
	allSettings map[shared.ProfileID]*notification.Settings,
	notifType notification.Type,
	genres []string,
) (map[shared.ProfileID][]string, error) {
	var relevanceIDs []shared.ProfileID
	for _, p := range profiles {
		cfg := allSettings[p.ID]
		if cfg == nil {
			continue
		}
		lvl := levelForType(cfg, notifType)
		if (lvl == notification.LevelRelevant || lvl == notification.LevelHighlyRelevant) && len(genres) > 0 {
			relevanceIDs = append(relevanceIDs, p.ID)
		}
	}
	if len(relevanceIDs) == 0 {
		return nil, nil
	}
	favs, err := s.analyzer.GetFavoriteGenresBatch(ctx, relevanceIDs, 3)
	if err != nil {
		slog.WarnContext(ctx, "notification: batch get favorite genres failed", slog.Any("err", err))
		return nil, err
	}
	return favs, nil
}

func shouldNotify(lvl notification.Level, genres, favorites []string) bool {
	if lvl == notification.LevelDisabled {
		return false
	}
	if lvl != notification.LevelRelevant && lvl != notification.LevelHighlyRelevant {
		return true
	}
	if len(genres) == 0 {
		return false
	}
	favSet := make(map[string]bool, len(favorites))
	for _, g := range favorites {
		favSet[g] = true
	}
	for _, g := range genres {
		if favSet[g] {
			return true
		}
	}
	return false
}
