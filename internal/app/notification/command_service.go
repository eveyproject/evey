package notification

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	libevent "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/notification"
	"github.com/evey-project/evey/internal/domain/shared"
	eveyi18n "github.com/evey-project/evey/internal/pkg/i18n"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// command_service.go holds the notification service type, its wiring
// (options + constructor), the event-subscription lifecycle (Register / Start
// / Stop) and the direct command API (mark-read, delete, settings, cleanup).
// The three cohesive sub-responsibilities live alongside it:
//   - batching.go       the "new media added" buffer + fan-out (owns s.mu state)
//   - event_handlers.go bus adapters for backup / plugin / media-request events
//   - push.go           persistence + web-push dispatch + subscription API

type CommandService struct {
	repo notification.CommandRepo
	// dedupReads is the one read the command flows need (episode fan-out
	// dedup); both halves come from the same concrete repository, the split
	// only narrows what each call site can reach (ADR-0009).
	dedupReads  notification.QueryRepo
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
		dedupReads: repo,
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
	s.events.Subscribe("media.item.removed", s.handleMediaItemRemoved)
	s.events.Subscribe("backup.failed", s.handleBackupFailed)
	s.events.Subscribe("plugin.error", s.handlePluginError)
	s.events.Subscribe("media.request.created", s.handleMediaRequestCreated)
	s.events.Subscribe("media.request.answered", s.handleRequestAnswered)
}

// handleMediaItemRemoved drops every notification about a title the moment
// it leaves the library: with no foreign key on media_item_id, they used to
// outlive it and open a media page that answers 404 (prod, 2026-09-03). The
// daily CleanupExpired sweep covers whatever a missed event leaves behind.
func (s *CommandService) handleMediaItemRemoved(ctx context.Context, ev shared.Event) {
	e, ok := ev.(libevent.MediaItemRemoved)
	if !ok || e.MediaItemID == "" {
		return
	}
	if err := s.repo.DeleteByMediaItem(ctx, e.MediaItemID); err != nil {
		slog.WarnContext(ctx, "notifications: delete for removed media item",
			slog.String("media_id", string(e.MediaItemID)), slog.Any("err", err))
	}
}

func (s *CommandService) Start(ctx context.Context) {
	s.shutdownCtx = ctx
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.ErrorContext(ctx, "recovered panic in notification service", slog.Any("error", r))
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

func (s *CommandService) MarkRead(ctx context.Context, id string, profileID shared.ProfileID) error {
	ctx, span := tracing.StartSpan(ctx, "app.notification.MarkRead")
	defer span.End()
	return s.repo.MarkRead(ctx, id, profileID)
}

func (s *CommandService) MarkAllRead(ctx context.Context, profileID shared.ProfileID) error {
	ctx, span := tracing.StartSpan(ctx, "app.notification.MarkAllRead")
	defer span.End()
	return s.repo.MarkAllRead(ctx, profileID)
}

func (s *CommandService) DeleteAll(ctx context.Context, profileID shared.ProfileID) error {
	ctx, span := tracing.StartSpan(ctx, "app.notification.DeleteAll")
	defer span.End()
	return s.repo.DeleteAll(ctx, profileID)
}

func (s *CommandService) UpdateSettings(ctx context.Context, settings *notification.Settings) error {
	ctx, span := tracing.StartSpan(ctx, "app.notification.UpdateSettings")
	defer span.End()
	if err := settings.Validate(); err != nil {
		return fmt.Errorf("invalid notification settings: %w", err)
	}
	return s.settings.Upsert(ctx, settings)
}

func (s *CommandService) UpdateGlobalSettings(ctx context.Context, settings *notification.Settings) error {
	ctx, span := tracing.StartSpan(ctx, "app.notification.UpdateGlobalSettings")
	defer span.End()
	if err := settings.Validate(); err != nil {
		return fmt.Errorf("invalid notification settings: %w", err)
	}
	return s.settings.UpsertGlobalDefaults(ctx, settings)
}

func (s *CommandService) CleanupExpired(ctx context.Context) error {
	ctx, span := tracing.StartSpan(ctx, "app.notification.CleanupExpired")
	defer span.End()
	return s.repo.CleanupExpired(ctx)
}

// StartCleanup runs an initial CleanupExpired then enters the periodic loop.
// The optional initDone channel(s) are closed after the first cleanup pass -
// tests pass one to wait deterministically instead of sleeping.
func (s *CommandService) StartCleanup(ctx context.Context, initDone ...chan<- struct{}) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.ErrorContext(ctx, "recovered panic in notification cleanup", "panic", r)
			}
		}()
		if err := s.repo.CleanupExpired(ctx); err != nil {
			slog.WarnContext(ctx, "notification cleanup", slog.Any("err", err))
		}
		for _, ch := range initDone {
			if ch != nil {
				close(ch)
			}
		}
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.repo.CleanupExpired(ctx); err != nil {
					slog.WarnContext(ctx, "notification cleanup", slog.Any("err", err))
				}
			}
		}
	}()
}
