package notification

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/evey-project/evey/internal/domain/backup"
	libevent "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/notification"
	"github.com/evey-project/evey/internal/domain/plugin"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
	eveyi18n "github.com/evey-project/evey/internal/pkg/i18n"
)

type batchEntry struct {
	seriesEvent *libevent.MediaItemAdded
	seasons     []libevent.MediaItemAdded
	episodes    []libevent.MediaItemAdded
	timer       *time.Timer
	createdAt   time.Time
}

type CommandService struct {
	repo        notification.Repository
	settings    notification.SettingsRepository
	analyzer    notification.WatchHistoryAnalyzer
	profiles    ProfileLister
	adminLister AdminProfileLister // optional: nil falls back to profiles.GetAll
	events      shared.EventBus
	push        notification.PushSender
	subs        notification.PushSubscriptionRepository
	translator  *eveyi18n.Translator

	mu          sync.Mutex
	dedupe      map[string]time.Time
	batches     map[string]*batchEntry
	batchDelay  time.Duration
	done        chan struct{}
	stopOnce    sync.Once
	shutdownCtx context.Context
}

const (
	defaultBatchDelay  = 5 * time.Second
	maxBatchEntries    = 1000
	episodeGroupWindow = 24 * time.Hour
)

type Option func(*CommandService)

func WithPush(push notification.PushSender, subs notification.PushSubscriptionRepository) Option {
	return func(s *CommandService) {
		s.push = push
		s.subs = subs
	}
}

func WithBatchDelay(d time.Duration) Option {
	return func(s *CommandService) { s.batchDelay = d }
}

// WithAdminLister lets wire inject a lookup that returns only admin-user
// profiles. Used to route security-critical notifications (plugin errors)
// so the admin gets paged instead of every user on the server.
func WithAdminLister(a AdminProfileLister) Option {
	return func(s *CommandService) { s.adminLister = a }
}

// WithTranslator enables locale-aware push notification titles. When nil, push
// titles fall back to the hardcoded EN strings.
func WithTranslator(t *eveyi18n.Translator) Option {
	return func(s *CommandService) { s.translator = t }
}

func NewCommandService(
	repo notification.Repository,
	settings notification.SettingsRepository,
	analyzer notification.WatchHistoryAnalyzer,
	profiles ProfileLister,
	events shared.EventBus,
	opts ...Option,
) *CommandService {
	s := &CommandService{
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
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *CommandService) Register() {
	s.events.Subscribe("media.item.added", s.handleMediaItemAdded)
	s.events.Subscribe("backup.failed", s.handleBackupFailed)
	s.events.Subscribe("plugin.error", s.handlePluginError)
}

func (s *CommandService) Start(ctx context.Context) {
	s.shutdownCtx = ctx
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

	s.emitNotifications(e.Type, e.Title, e.SeriesTitle, string(e.MediaItemID), "", e.Genres, nil)
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
			if ob := s.batches[oldestKey]; ob != nil && ob.timer != nil {
				ob.timer.Stop()
			}
			delete(s.batches, oldestKey)
			slog.Warn("notification batch map full, evicted oldest entry", slog.String("key", oldestKey))
		}
		batch = &batchEntry{createdAt: time.Now()}
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
			title, push := batchedAlbumTitle(batch.seriesEvent.Title, episodeCount)
			s.emitNotifications("album", title, "", string(batch.seriesEvent.MediaItemID), "", batch.seriesEvent.Genres, &push)
			return
		}
		seriesKey := seriesKeyFor(string(batch.seriesEvent.LibraryID), batch.seriesEvent.Title)
		title, push := batchedSeriesTitle(batch.seriesEvent.Title, seasonCount, episodeCount)
		s.emitNotifications("series", title, "", string(batch.seriesEvent.MediaItemID), seriesKey, batch.seriesEvent.Genres, &push)
		return
	}

	// Seasons without series event (e.g. new season added to existing series)
	if seasonCount > 0 && episodeCount > 0 {
		season := batch.seasons[0]
		seriesTitle := season.SeriesTitle
		if seriesTitle == "" {
			seriesTitle = season.Title
		}
		seriesKey := seriesKeyFor(string(season.LibraryID), season.SeriesTitle)
		title, push := batchedSeasonTitle(seriesTitle, season.Title, episodeCount)
		s.emitNotifications("season", title, "", string(season.MediaItemID), seriesKey, season.Genres, &push)
		return
	}

	// Season(s) alone — emit each one
	if seasonCount > 0 {
		for _, season := range batch.seasons {
			seriesKey := seriesKeyFor(string(season.LibraryID), season.SeriesTitle)
			s.emitNotifications(season.Type, season.Title, season.SeriesTitle, string(season.MediaItemID), seriesKey, season.Genres, nil)
		}
		return
	}

	// Just episodes
	if episodeCount == 1 {
		ep := batch.episodes[0]
		seriesKey := seriesKeyFor(string(ep.LibraryID), ep.SeriesTitle)
		s.emitNotifications(ep.Type, ep.Title, ep.SeriesTitle, string(ep.MediaItemID), seriesKey, ep.Genres, nil, 1)
		return
	}

	if episodeCount > 1 {
		first := batch.episodes[0]
		seriesKey := seriesKeyFor(string(first.LibraryID), first.SeriesTitle)
		title, push := batchedEpisodesTitle(first.SeriesTitle, episodeCount)
		s.emitNotifications("episode", title, "", string(first.MediaItemID), seriesKey, first.Genres, &push, episodeCount)
	}
}

func (s *CommandService) emitNotifications(mediaType, title, seriesTitle, mediaItemID, seriesKey string, genres []string, pushOverride *pushSpec, episodeCounts ...int) {
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
			slog.Warn("notification: list profiles failed", slog.Any("err", err))
		}
		return
	}

	profileIDs := make([]string, len(profiles))
	for i, p := range profiles {
		profileIDs[i] = string(p.ID)
	}
	allSettings, err := s.settings.GetByProfiles(ctx, profileIDs)
	if err != nil {
		slog.Warn("notification: get settings batch failed", slog.Any("err", err))
		return
	}

	favoritesByProfile, err := s.fetchFavoriteGenres(ctx, profiles, allSettings, notifType, genres)
	if err != nil {
		return
	}

	initialEpCount := 0
	if len(episodeCounts) > 0 {
		initialEpCount = episodeCounts[0]
	}

	expires := time.Now().AddDate(0, 0, defaultRetentionDays)
	since := time.Now().Add(-episodeGroupWindow)
	for _, p := range profiles {
		cfg := allSettings[string(p.ID)]
		if cfg == nil {
			continue
		}
		lvl := levelForType(cfg, notifType)
		if !shouldNotify(lvl, genres, favoritesByProfile[string(p.ID)]) {
			continue
		}

		// For episode notifications with a series key, check for recent series/season
		// notifications (skip) or recent episode notification (group/update).
		if notifType == notification.TypeNewEpisode && seriesKey != "" {
			if s.shouldSkipEpisodeNotif(ctx, string(p.ID), seriesKey, since, notifTitle, push, p.Locale, initialEpCount) {
				continue
			}
		}

		opts := []notification.NotificationOption{
			notification.WithNotificationMediaItem(shared.MediaItemID(mediaItemID)),
			notification.WithNotificationLevel(lvl),
			notification.WithNotificationExpiresAt(&expires),
		}
		if seriesKey != "" {
			opts = append(opts, notification.WithRelatedMediaID(shared.MediaItemID(seriesKey)))
		}
		if notifType == notification.TypeNewEpisode && initialEpCount > 0 {
			opts = append(opts, notification.WithEpisodeCount(initialEpCount))
		}
		n, err := notification.NewNotification(uuid.New().String(), p.ID, notifType, notifTitle, opts...)
		if err != nil {
			slog.Warn("notification build failed", slog.String("profileID", string(p.ID)), slog.Any("err", err))
			continue
		}
		s.persistAndDispatch(ctx, n, push.resolveTitle(s.translator, p.Locale))
	}
}

// shouldSkipEpisodeNotif returns true if the episode notification should be
// suppressed (series/season notif already sent recently) or updated in-place
// (an existing grouped episode notif is within the grouping window).
// When it updates an existing notification, it also returns true so the caller
// skips creating a new one.
// initialCount is the number of episodes this batch represents (1 for a single
// episode, N for a pre-batched group). It is used to correctly compute the
// accumulated count when merging into an existing notification.
func (s *CommandService) shouldSkipEpisodeNotif(ctx context.Context, profileID, seriesKey string, since time.Time, notifTitle string, push pushSpec, locale string, initialCount int) bool {
	// Case A/B: series or season notification already sent for this series → skip.
	parentTypes := []notification.Type{notification.TypeNewSeries, notification.TypeNewSeason}
	parentNotifs, err := s.repo.FindRecentByProfileTypeAndSeries(ctx, profileID, parentTypes, seriesKey, since)
	if err != nil {
		slog.Warn("notification: find recent series/season failed", slog.Any("err", err))
		return false // fail open: create the notification
	}
	if len(parentNotifs) > 0 {
		return true
	}

	// Case C: existing grouped episode notification within window → update it.
	epTypes := []notification.Type{notification.TypeNewEpisode}
	existingEps, err := s.repo.FindRecentByProfileTypeAndSeries(ctx, profileID, epTypes, seriesKey, since)
	if err != nil {
		slog.Warn("notification: find recent episodes failed", slog.Any("err", err))
		return false
	}
	if len(existingEps) == 0 {
		return false
	}

	// Update the most recent one.
	existing := existingEps[0]
	addCount := initialCount
	if addCount < 1 {
		addCount = 1
	}
	prevCount := existing.EpisodeCount
	if prevCount < 1 {
		prevCount = 1 // treat the original notification as representing at least 1 episode
	}
	newCount := prevCount + addCount

	// Re-derive the series name from the stored title so we can rebuild it.
	seriesName := extractSeriesName(existing.Title)
	newTitle, newPush := batchedEpisodesTitle(seriesName, newCount)
	existing.Title = "new_episode:" + newTitle
	existing.EpisodeCount = newCount
	if updateErr := s.repo.Update(ctx, existing); updateErr != nil {
		slog.Warn("notification: update grouped episode failed", slog.Any("err", updateErr))
		return false // fail open
	}
	// Re-send push for the updated notification.
	s.sendPush(ctx, profileID, newPush.resolveTitle(s.translator, locale), existing.Message)
	return true
}

func (s *CommandService) fetchFavoriteGenres(
	ctx context.Context,
	profiles []*user.Profile,
	allSettings map[string]*notification.Settings,
	notifType notification.Type,
	genres []string,
) (map[string][]string, error) {
	var relevanceIDs []string
	for _, p := range profiles {
		cfg := allSettings[string(p.ID)]
		if cfg == nil {
			continue
		}
		lvl := levelForType(cfg, notifType)
		if (lvl == notification.LevelRelevant || lvl == notification.LevelHighlyRelevant) && len(genres) > 0 {
			relevanceIDs = append(relevanceIDs, string(p.ID))
		}
	}
	if len(relevanceIDs) == 0 {
		return nil, nil
	}
	favs, err := s.analyzer.GetFavoriteGenresBatch(ctx, relevanceIDs, 3)
	if err != nil {
		slog.Warn("notification: batch get favorite genres failed", slog.Any("err", err))
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

func (s *CommandService) persistAndDispatch(ctx context.Context, n *notification.Notification, pushTitle string) {
	if createErr := s.repo.Create(ctx, n); createErr != nil {
		slog.Warn("create notification failed", slog.String("profileID", string(n.ProfileID)), slog.Any("err", createErr))
		return
	}
	s.sendPush(ctx, string(n.ProfileID), pushTitle, n.Message)
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

func (s *CommandService) sendPush(ctx context.Context, profileID, title, message string) {
	if s.push == nil || s.subs == nil {
		return
	}
	subs, err := s.subs.GetByProfile(ctx, profileID)
	if err != nil {
		slog.Warn("webpush: get subscriptions failed", slog.String("profileID", profileID), slog.Any("err", err))
		return
	}
	for _, sub := range subs {
		if sendErr := s.push.Send(ctx, sub, title, message, nil); sendErr != nil {
			slog.Warn("webpush: send failed", slog.String("profileID", profileID), slog.String("endpoint", sub.Endpoint), slog.Any("err", sendErr))
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
	push := pushSpec{key: "notifications.push.backup_failed.title", fallback: title}
	for _, p := range profiles {
		n, err := notification.NewNotification(uuid.New().String(), p.ID, notification.TypeBackupFailed, title,
			notification.WithNotificationMessage(e.Error),
			notification.WithNotificationLevel(notification.LevelAll),
			notification.WithNotificationExpiresAt(&expires),
		)
		if err != nil {
			slog.Warn("notification build failed", slog.String("profileID", string(p.ID)), slog.Any("err", err))
			continue
		}
		s.persistAndDispatch(ctx, n, push.resolveTitle(s.translator, p.Locale))
	}
}

func (s *CommandService) handlePluginError(parentCtx context.Context, ev shared.Event) {
	// Recover locally so a deterministic panic on a single event does not
	// trigger the bus retry loop and burn queue slots at full speed.
	defer func() {
		if rv := recover(); rv != nil {
			var pluginID string
			if e, ok := ev.(plugin.PluginError); ok {
				pluginID = e.PluginID
			}
			slog.Error("handlePluginError panic recovered",
				slog.Any("panic", rv),
				slog.String("plugin_id", pluginID))
		}
	}()

	ctx, cancel := context.WithTimeout(parentCtx, 30*time.Second)
	defer cancel()
	e, ok := ev.(plugin.PluginError)
	if !ok {
		return
	}

	profiles, err := s.listAdminTargetProfiles(ctx)
	if err != nil {
		slog.Warn("notification: list admin profiles for plugin.error", slog.Any("err", err))
		return
	}

	title := "Plugin error"
	push := pushSpec{key: "notifications.push.plugin_error.title", fallback: title}
	if e.PluginID != "" {
		title = "Plugin error: " + e.PluginID
		push = pushSpec{
			key:      "notifications.push.plugin_error_id.title",
			params:   map[string]string{"plugin": e.PluginID},
			fallback: title,
		}
	}
	msg := e.ErrorMessage
	if msg == "" {
		msg = "plugin reported an error"
	}
	expires := time.Now().AddDate(0, 0, defaultRetentionDays)
	for _, p := range profiles {
		n, err := notification.NewNotification(uuid.New().String(), p.ID, notification.TypePluginError, title,
			notification.WithNotificationMessage(msg),
			notification.WithNotificationLevel(notification.LevelAll),
			notification.WithNotificationExpiresAt(&expires),
		)
		if err != nil {
			slog.Warn("notification build failed", slog.String("profileID", string(p.ID)), slog.Any("err", err))
			continue
		}
		s.persistAndDispatch(ctx, n, push.resolveTitle(s.translator, p.Locale))
	}
	slog.Info("plugin error notified",
		slog.String("plugin_id", e.PluginID),
		slog.Int("admins", len(profiles)))
}

// listAdminTargetProfiles prefers the optional AdminProfileLister so
// plugin errors page only admin users. Falls back to all profiles when
// no admin lookup is wired (keeps dev/test setups working).
func (s *CommandService) listAdminTargetProfiles(ctx context.Context) ([]*user.Profile, error) {
	if s.adminLister != nil {
		return s.adminLister.ListAdminProfiles(ctx)
	}
	return s.profiles.GetAll(ctx)
}

func (s *CommandService) MarkRead(ctx context.Context, id, profileID string) error {
	return s.repo.MarkRead(ctx, id, profileID)
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

func (s *CommandService) UpsertPushSubscription(ctx context.Context, userID, profileID, endpoint, p256dh, auth string) error {
	if s.subs == nil {
		return nil
	}
	return s.subs.Upsert(ctx, userID, profileID, endpoint, p256dh, auth)
}

func (s *CommandService) DeletePushByEndpoint(ctx context.Context, userID, endpoint string) error {
	if s.subs == nil {
		return nil
	}
	return s.subs.DeleteByEndpoint(ctx, userID, endpoint)
}

func (s *CommandService) DeletePushByUser(ctx context.Context, userID string) error {
	if s.subs == nil {
		return nil
	}
	return s.subs.DeleteByUser(ctx, userID)
}

