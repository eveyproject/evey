package notification

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	libevent "github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/notification"
	"github.com/gravity-zero/evey/internal/domain/shared"
)

type CommandService struct {
	repo     notification.Repository
	settings notification.SettingsRepository
	analyzer notification.WatchHistoryAnalyzer
	profiles ProfileLister
	events   shared.EventBus
	push notification.PushSender
	subs notification.PushSubscriptionRepository

	mu       sync.Mutex
	dedupe   map[string]time.Time
	done     chan struct{}
	stopOnce sync.Once
}

func NewCommandService(
	repo notification.Repository,
	settings notification.SettingsRepository,
	analyzer notification.WatchHistoryAnalyzer,
	profiles ProfileLister,
	events shared.EventBus,
) *CommandService {
	return &CommandService{
		repo:     repo,
		settings: settings,
		analyzer: analyzer,
		profiles: profiles,
		events:   events,
		dedupe:   make(map[string]time.Time),
		done:     make(chan struct{}),
	}
}

func (s *CommandService) Register() {
	s.events.Subscribe("media.item.added", s.handleMediaItemAdded)
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
}

func (s *CommandService) handleMediaItemAdded(parentCtx context.Context, ev shared.Event) {
	ctx, cancel := context.WithTimeout(parentCtx, 30*time.Second)
	defer cancel()
	e, ok := ev.(libevent.MediaItemAdded)
	if !ok {
		return
	}

	dedupeKey := fmt.Sprintf("%s:%s", e.Type, e.MediaItemID)
	s.mu.Lock()
	if last, exists := s.dedupe[dedupeKey]; exists && time.Since(last) < dedupeWindow {
		s.mu.Unlock()
		return
	}
	s.dedupe[dedupeKey] = time.Now()
	s.mu.Unlock()

	notifType, title := typeAndTitle(e.Type, e.Title, e.SeriesTitle)
	if notifType == "" {
		return
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

	// Pre-fetch favorite genres for profiles needing relevance check (avoids N+1)
	var relevanceIDs []string
	for _, p := range profiles {
		cfg := allSettings[p.ID]
		if cfg == nil {
			continue
		}
		lvl := levelForType(cfg, notifType)
		if (lvl == notification.LevelRelevant || lvl == notification.LevelHighlyRelevant) && len(e.Genres) > 0 {
			relevanceIDs = append(relevanceIDs, p.ID)
		}
	}
	var favoritesByProfile map[string][]string
	if len(relevanceIDs) > 0 {
		var batchErr error
		favoritesByProfile, batchErr = s.analyzer.GetFavoriteGenresBatch(ctx, relevanceIDs, 3)
		if batchErr != nil {
			slog.Warn("notification: batch get favorite genres failed", slog.Any("err", batchErr))
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
			if len(e.Genres) == 0 {
				continue
			}
			favs := favoritesByProfile[p.ID]
			favSet := make(map[string]bool, len(favs))
			for _, g := range favs {
				favSet[g] = true
			}
			score := 0
			for _, g := range e.Genres {
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
			Title:       title,
			Message:     "",
			MediaItemID: e.MediaItemID,
			Level:       lvl,
			CreatedAt:   time.Now(),
			ExpiresAt:   &expires,
		}
		if err := s.repo.Create(ctx, n); err != nil {
			slog.Warn("create notification failed", slog.String("profileID", p.ID), slog.Any("err", err))
			continue
		}
		if s.push != nil && s.subs != nil {
			subs, err := s.subs.GetByProfile(ctx, n.ProfileID)
			if err != nil {
				slog.Warn("webpush: get subscriptions failed", slog.String("profileID", n.ProfileID), slog.Any("err", err))
			} else {
				for _, sub := range subs {
					if err := s.push.Send(ctx, sub, n.Title, n.Message, nil); err != nil {
						slog.Warn("webpush: send failed", slog.String("profileID", n.ProfileID), slog.String("endpoint", sub.Endpoint), slog.Any("err", err))
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
		if err := s.events.Publish(ctx, evt); err != nil {
			slog.Warn("publish notification.created", slog.Any("err", err))
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

