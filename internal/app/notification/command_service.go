package notification

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gravity-zero/evey/internal/domain/backup"
	libevent "github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/notification"
	"github.com/gravity-zero/evey/internal/domain/shared"
)

type batchEntry struct {
	seriesEvent *libevent.MediaItemAdded
	seasons     []libevent.MediaItemAdded
	episodes    []libevent.MediaItemAdded
	timer       *time.Timer
}

type CommandService struct {
	repo     notification.Repository
	settings notification.SettingsRepository
	analyzer notification.WatchHistoryAnalyzer
	profiles ProfileLister
	events   shared.EventBus
	push     notification.PushSender
	subs     notification.PushSubscriptionRepository

	mu         sync.Mutex
	dedupe     map[string]time.Time
	batches    map[string]*batchEntry
	batchDelay time.Duration
	done       chan struct{}
	stopOnce   sync.Once
}

const defaultBatchDelay = 5 * time.Second

func NewCommandService(
	repo notification.Repository,
	settings notification.SettingsRepository,
	analyzer notification.WatchHistoryAnalyzer,
	profiles ProfileLister,
	events shared.EventBus,
) *CommandService {
	return &CommandService{
		repo:       repo,
		settings:   settings,
		analyzer:   analyzer,
		profiles:   profiles,
		events:     events,
		dedupe:     make(map[string]time.Time),
		batches:    make(map[string]*batchEntry),
		batchDelay: defaultBatchDelay,
		done:       make(chan struct{}),
	}
}

func (s *CommandService) WithBatchDelay(d time.Duration) *CommandService {
	s.batchDelay = d
	return s
}

func (s *CommandService) Register() {
	s.events.Subscribe("media.item.added", s.handleMediaItemAdded)
	s.events.Subscribe("backup.failed", s.handleBackupFailed)
}

func (s *CommandService) WithPush(push notification.PushSender, subs notification.PushSubscriptionRepository) *CommandService {
	s.push = push
	s.subs = subs
	return s
}

func (s *CommandService) Start(ctx context.Context) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("recovered panic in notification service", slog.Any("error", r))
			}
		}()
		ticker := time.NewTicker(dedupeWindow)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-s.done:
				return
			case <-ticker.C:
				s.mu.Lock()
				for k, t := range s.dedupe {
					if time.Since(t) >= dedupeWindow {
						delete(s.dedupe, k)
					}
				}
				s.mu.Unlock()
			}
		}
	}()
}

func (s *CommandService) Stop() {
	s.stopOnce.Do(func() { close(s.done) })
	s.mu.Lock()
	for _, b := range s.batches {
		if b.timer != nil {
			b.timer.Stop()
		}
	}
	s.mu.Unlock()
}

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
	if e.Type == "photo" || e.Type == "image" {
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

	// Albums get batched by album title (tracks amalgamated)
	if e.Type == "album" || (e.Type == "track" && e.SeriesTitle != "") {
		s.bufferSeriesEvent(e)
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	s.emitNotifications(e.Type, e.Title, e.SeriesTitle, e.MediaItemID, e.Genres, "")
}

func isBatchable(e libevent.MediaItemAdded) bool {
	switch e.Type {
	case "series", "season":
		return true
	case "episode":
		return e.SeriesTitle != ""
	}
	return false
}

func (s *CommandService) batchKey(e libevent.MediaItemAdded) string {
	// Albums use their own title as key, series use SeriesTitle
	if e.Type == "album" || e.Type == "track" {
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
		batch = &batchEntry{}
		s.batches[key] = batch
	}

	switch e.Type {
	case "series", "album":
		batch.seriesEvent = &e
	case "season":
		batch.seasons = append(batch.seasons, e)
	case "episode", "track":
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

	seasonCount := len(batch.seasons)
	episodeCount := len(batch.episodes)

	// Full series (series event present)
	if batch.seriesEvent != nil {
		if batch.seriesEvent.Type == "album" {
			title, pushTitle := batchedAlbumTitle(batch.seriesEvent.Title, episodeCount)
			s.emitNotifications("album", title, "", batch.seriesEvent.MediaItemID, batch.seriesEvent.Genres, pushTitle)
			return
		}
		title, pushTitle := batchedSeriesTitle(batch.seriesEvent.Title, seasonCount, episodeCount)
		s.emitNotifications("series", title, "", batch.seriesEvent.MediaItemID, batch.seriesEvent.Genres, pushTitle)
		return
	}

	// Seasons without series event (e.g. new season added to existing series)
	if seasonCount > 0 && episodeCount > 0 {
		season := batch.seasons[0]
		seriesTitle := season.SeriesTitle
		if seriesTitle == "" {
			seriesTitle = season.Title
		}
		title, pushTitle := batchedSeasonTitle(seriesTitle, season.Title, episodeCount)
		s.emitNotifications("season", title, "", season.MediaItemID, season.Genres, pushTitle)
		return
	}

	// Season(s) alone — emit each one
	if seasonCount > 0 {
		for _, season := range batch.seasons {
			s.emitNotifications(season.Type, season.Title, season.SeriesTitle, season.MediaItemID, season.Genres, "")
		}
		return
	}

	// Just episodes
	if episodeCount == 1 {
		ep := batch.episodes[0]
		s.emitNotifications(ep.Type, ep.Title, ep.SeriesTitle, ep.MediaItemID, ep.Genres, "")
		return
	}

	if episodeCount > 1 {
		first := batch.episodes[0]
		title, pushTitle := batchedEpisodesTitle(first.SeriesTitle, episodeCount)
		s.emitNotifications("episode", title, "", first.MediaItemID, first.Genres, pushTitle)
	}
}

func (s *CommandService) emitNotifications(mediaType, title, seriesTitle, mediaItemID string, genres []string, pushTitleOverride string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	notifType, notifTitle, pushTitle := typeAndTitle(mediaType, title, seriesTitle)
	if notifType == "" {
		return
	}
	if pushTitleOverride != "" {
		pushTitle = pushTitleOverride
	}

	profiles, err := s.profiles.GetAll(ctx)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			slog.Warn("notification: list profiles failed", slog.Any("err", err))
		}
		return
	}

	profileIDs := make([]string, len(profiles))
	for i, p := range profiles {
		profileIDs[i] = p.ID
	}
	allSettings, err := s.settings.GetByProfiles(ctx, profileIDs)
	if err != nil {
		slog.Warn("notification: get settings batch failed", slog.Any("err", err))
		return
	}

	var relevanceIDs []string
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
	var favoritesByProfile map[string][]string
	if len(relevanceIDs) > 0 {
		favoritesByProfile, err = s.analyzer.GetFavoriteGenresBatch(ctx, relevanceIDs, 3)
		if err != nil {
			slog.Warn("notification: batch get favorite genres failed", slog.Any("err", err))
			return
		}
	}

	expires := time.Now().AddDate(0, 0, defaultRetentionDays)
	for _, p := range profiles {
		cfg := allSettings[p.ID]
		if cfg == nil {
			continue
		}
		lvl := levelForType(cfg, notifType)
		if lvl == notification.LevelDisabled {
			continue
		}
		if lvl == notification.LevelRelevant || lvl == notification.LevelHighlyRelevant {
			if len(genres) == 0 {
				continue
			}
			favs := favoritesByProfile[p.ID]
			favSet := make(map[string]bool, len(favs))
			for _, g := range favs {
				favSet[g] = true
			}
			score := 0
			for _, g := range genres {
				if favSet[g] {
					score++
				}
			}
			if score == 0 {
				continue
			}
		}
		n := &notification.Notification{
			ID:          uuid.New().String(),
			ProfileID:   p.ID,
			Type:        notifType,
			Title:       notifTitle,
			Message:     "",
			MediaItemID: mediaItemID,
			Level:       lvl,
			CreatedAt:   time.Now(),
			ExpiresAt:   &expires,
		}
		if createErr := s.repo.Create(ctx, n); createErr != nil {
			slog.Warn("create notification failed", slog.String("profileID", p.ID), slog.Any("err", createErr))
			continue
		}
		if s.push != nil && s.subs != nil {
			subs, subErr := s.subs.GetByProfile(ctx, n.ProfileID)
			if subErr != nil {
				slog.Warn("webpush: get subscriptions failed", slog.String("profileID", n.ProfileID), slog.Any("err", subErr))
			} else {
				for _, sub := range subs {
					if sendErr := s.push.Send(ctx, sub, pushTitle, n.Message, nil); sendErr != nil {
						slog.Warn("webpush: send failed", slog.String("profileID", n.ProfileID), slog.String("endpoint", sub.Endpoint), slog.Any("err", sendErr))
					}
				}
			}
		}
		evt := notification.NotificationCreated{
			NotificationID: n.ID,
			ProfileID:      n.ProfileID,
			Type:           n.Type,
			Title:          n.Title,
			MediaItemID:    n.MediaItemID,
			OccurredAt:     n.CreatedAt,
		}
		if pubErr := s.events.Publish(ctx, evt); pubErr != nil {
			slog.Warn("publish notification.created", slog.Any("err", pubErr))
		}
	}
}

func (s *CommandService) handleBackupFailed(parentCtx context.Context, ev shared.Event) {
	ctx, cancel := context.WithTimeout(parentCtx, 30*time.Second)
	defer cancel()
	e, ok := ev.(backup.BackupFailed)
	if !ok {
		return
	}

	profiles, err := s.profiles.GetAll(ctx)
	if err != nil {
		slog.Warn("notification: list profiles for backup.failed", slog.Any("err", err))
		return
	}

	expires := time.Now().AddDate(0, 0, defaultRetentionDays)
	title := "Scheduled Backup Failed"
	for _, p := range profiles {
		n := &notification.Notification{
			ID:        uuid.New().String(),
			ProfileID: p.ID,
			Type:      notification.TypeBackupFailed,
			Title:     title,
			Message:   e.Error,
			Level:     notification.LevelAll,
			CreatedAt: time.Now(),
			ExpiresAt: &expires,
		}
		if err := s.repo.Create(ctx, n); err != nil {
			slog.Warn("create backup failure notification", slog.String("profileID", p.ID), slog.Any("err", err))
			continue
		}
		if s.push != nil && s.subs != nil {
			subs, subErr := s.subs.GetByProfile(ctx, n.ProfileID)
			if subErr != nil {
				slog.Warn("webpush: get subscriptions for backup.failed", slog.String("profileID", n.ProfileID), slog.Any("err", subErr))
			} else {
				for _, sub := range subs {
					if pushErr := s.push.Send(ctx, sub, n.Title, n.Message, nil); pushErr != nil {
						slog.Warn("webpush: send backup.failed", slog.String("profileID", n.ProfileID), slog.Any("err", pushErr))
					}
				}
			}
		}
		evt := notification.NotificationCreated{
			NotificationID: n.ID,
			ProfileID:      n.ProfileID,
			Type:           n.Type,
			Title:          n.Title,
			OccurredAt:     n.CreatedAt,
		}
		if pubErr := s.events.Publish(ctx, evt); pubErr != nil {
			slog.Warn("publish notification.created for backup.failed", slog.Any("err", pubErr))
		}
	}
}

func (s *CommandService) MarkRead(ctx context.Context, id string) error {
	return s.repo.MarkRead(ctx, id)
}

func (s *CommandService) MarkAllRead(ctx context.Context, profileID string) error {
	return s.repo.MarkAllRead(ctx, profileID)
}

func (s *CommandService) DeleteAll(ctx context.Context, profileID string) error {
	return s.repo.DeleteAll(ctx, profileID)
}

func (s *CommandService) UpdateSettings(ctx context.Context, settings *notification.Settings) error {
	if err := settings.Validate(); err != nil {
		return fmt.Errorf("invalid notification settings: %w", err)
	}
	return s.settings.Upsert(ctx, settings)
}

func (s *CommandService) UpdateGlobalSettings(ctx context.Context, settings *notification.Settings) error {
	if err := settings.Validate(); err != nil {
		return fmt.Errorf("invalid notification settings: %w", err)
	}
	return s.settings.UpsertGlobalDefaults(ctx, settings)
}

func (s *CommandService) CleanupExpired(ctx context.Context) error {
	return s.repo.CleanupExpired(ctx)
}

func (s *CommandService) StartCleanup(ctx context.Context) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("recovered panic in notification cleanup", "panic", r)
			}
		}()
		if err := s.repo.CleanupExpired(ctx); err != nil {
			slog.Warn("notification cleanup", slog.Any("err", err))
		}
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.repo.CleanupExpired(ctx); err != nil {
					slog.Warn("notification cleanup", slog.Any("err", err))
				}
			}
		}
	}()
}

