package notification

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	libevent "github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/notification"
	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/gravity-zero/evey/internal/domain/user"
)

const dedupeWindow = 5 * time.Minute
const defaultRetentionDays = 30

type ProfileLister interface {
	GetAll(ctx context.Context) ([]*user.Profile, error)
}

type Service struct {
	repo     notification.Repository
	settings notification.SettingsRepository
	analyzer notification.WatchHistoryAnalyzer
	profiles ProfileLister
	events   shared.EventBus

	mu     sync.Mutex
	dedupe map[string]time.Time // key: "type:parentID" → last sent
}

func NewService(
	repo notification.Repository,
	settings notification.SettingsRepository,
	analyzer notification.WatchHistoryAnalyzer,
	profiles ProfileLister,
	events shared.EventBus,
) *Service {
	s := &Service{
		repo:     repo,
		settings: settings,
		analyzer: analyzer,
		profiles: profiles,
		events:   events,
		dedupe:   make(map[string]time.Time),
	}
	events.Subscribe("media.item.added", s.handleMediaItemAdded)
	return s
}

func (s *Service) handleMediaItemAdded(ctx context.Context, ev shared.Event) {
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

	notifType, title := typeAndTitle(e.Type, e.Title)
	if notifType == "" {
		return
	}

	profiles, err := s.profiles.GetAll(ctx)
	if err != nil {
		return
	}

	expires := time.Now().AddDate(0, 0, defaultRetentionDays)
	for _, p := range profiles {
		cfg, err := s.settings.GetByProfile(ctx, p.ID)
		if err != nil {
			continue
		}
		lvl := levelForType(cfg, notifType)
		if lvl == notification.LevelDisabled {
			continue
		}
		if lvl == notification.LevelRelevant || lvl == notification.LevelHighlyRelevant {
			if len(e.Genres) == 0 {
				// genres unknown yet (e.g. fresh scan before metadata refresh) — skip filter
				break
			}
			score, err := s.analyzer.CalculateRelevanceScore(ctx, p.ID, e.Genres)
			if err != nil || score == 0 {
				continue
			}
		}
		n := &notification.Notification{
			ID:          uuid.New().String(),
			ProfileID:   p.ID,
			Type:        notifType,
			Title:       title,
			Message:     fmt.Sprintf("%s added to your library", e.Title),
			MediaItemID: e.MediaItemID,
			Level:       lvl,
			CreatedAt:   time.Now(),
			ExpiresAt:   &expires,
		}
		_ = s.repo.Create(ctx, n)
	}
}

func typeAndTitle(mediaType, title string) (notification.Type, string) {
	switch mediaType {
	case "movie":
		return notification.TypeNewMovie, "New Movie: " + title
	case "episode":
		return notification.TypeNewEpisode, "New Episode: " + title
	case "season":
		return notification.TypeNewSeason, "New Season: " + title
	case "series":
		return notification.TypeNewSeries, "New Series: " + title
	case "album":
		return notification.TypeNewAlbum, "New Album: " + title
	}
	return "", ""
}

func levelForType(cfg *notification.Settings, t notification.Type) notification.Level {
	switch t {
	case notification.TypeNewMovie:
		return cfg.MovieLevel
	case notification.TypeNewEpisode, notification.TypeNewSeason, notification.TypeNewSeries:
		return cfg.SeriesLevel
	case notification.TypeNewAlbum:
		return cfg.MusicLevel
	}
	return notification.LevelAll
}

func (s *Service) List(ctx context.Context, profileID string, unreadOnly bool, limit, offset int) ([]*notification.Notification, error) {
	return s.repo.ListByProfile(ctx, profileID, unreadOnly, limit, offset)
}

func (s *Service) MarkRead(ctx context.Context, id string) error {
	return s.repo.MarkRead(ctx, id)
}

func (s *Service) MarkAllRead(ctx context.Context, profileID string) error {
	return s.repo.MarkAllRead(ctx, profileID)
}

func (s *Service) UnreadCount(ctx context.Context, profileID string) (int, error) {
	return s.repo.UnreadCount(ctx, profileID)
}

func (s *Service) DeleteAll(ctx context.Context, profileID string) error {
	return s.repo.DeleteAll(ctx, profileID)
}

func (s *Service) GetSettings(ctx context.Context, profileID string) (*notification.Settings, error) {
	return s.settings.GetByProfile(ctx, profileID)
}

func (s *Service) UpdateSettings(ctx context.Context, settings *notification.Settings) error {
	return s.settings.Upsert(ctx, settings)
}

func (s *Service) GetGlobalSettings(ctx context.Context) (*notification.Settings, error) {
	return s.settings.GetGlobalDefaults(ctx)
}

func (s *Service) UpdateGlobalSettings(ctx context.Context, settings *notification.Settings) error {
	return s.settings.UpsertGlobalDefaults(ctx, settings)
}

func (s *Service) CleanupExpired(ctx context.Context) error {
	return s.repo.CleanupExpired(ctx)
}
