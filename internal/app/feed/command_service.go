package feed

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/domain/feed"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type CommandServiceOption func(*CommandService)

func CommandWithEventBus(bus shared.EventBus) CommandServiceOption {
	return func(s *CommandService) {
		s.bus = bus
		bus.Subscribe("library.created", func(ctx context.Context, ev shared.Event) {
			e, ok := ev.(library.LibraryCreated)
			if !ok {
				return
			}
			if err := s.EnsureLibraryFeedConfig(ctx, e.LibraryID, e.Name); err != nil {
				slog.WarnContext(ctx, "auto-create library feed config", slog.String("library", string(e.LibraryID)), slog.Any("err", err))
			}
		})
		bus.Subscribe("library.deleted", func(ctx context.Context, ev shared.Event) {
			e, ok := ev.(library.LibraryDeleted)
			if !ok {
				return
			}
			if err := s.DeleteLibraryFeedConfig(ctx, e.LibraryID); err != nil {
				slog.WarnContext(ctx, "auto-delete library feed config", slog.String("library", string(e.LibraryID)), slog.Any("err", err))
			}
		})
		// A plugin's home rows die with the plugin, as a library's do: nothing
		// listened here before, so every reinstall of a widget left its old
		// plugin:<id> row in each profile, and the homes rendered the ghosts.
		bus.Subscribe(plugin.PluginUninstalled{}.EventName(), func(ctx context.Context, ev shared.Event) {
			e, ok := ev.(plugin.PluginUninstalled)
			if !ok {
				return
			}
			if err := s.DeletePluginFeedConfig(ctx, e.PluginID); err != nil {
				slog.WarnContext(ctx, "auto-delete plugin feed config", slog.String("plugin", e.PluginID), slog.Any("err", err))
			}
		})
	}
}

type CommandService struct {
	commands feed.ConfigCommandRepo
	queries  feed.ConfigQueryRepo
	bus      shared.EventBus
	// ensureMu serialises EnsureLibraryFeedConfig. It reads the whole admin
	// config to derive the next display_order and then writes it back, and the
	// event bus can fire library.created twice in quick succession (initial
	// setup with several libraries, two admins at once). Two concurrent runs
	// read the same snapshot, compute the same maxOrder+1, and the upsert has
	// no constraint on display_order - so both rows land on the same position
	// and the admin feed order becomes arbitrary between them.
	ensureMu sync.Mutex
}

func NewCommandService(commands feed.ConfigCommandRepo, queries feed.ConfigQueryRepo, opts ...CommandServiceOption) *CommandService {
	s := &CommandService{commands: commands, queries: queries}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *CommandService) publish(ctx context.Context, ev shared.Event) {
	if s.bus == nil {
		return
	}
	if err := s.bus.Publish(ctx, ev); err != nil {
		slog.WarnContext(ctx, "publish feed event", slog.String("event", ev.EventName()), slog.Any("err", err))
	}
}

func (s *CommandService) UpdateAdminConfig(ctx context.Context, cfg *feed.AdminConfig) error {
	ctx, span := tracing.StartSpan(ctx, "app.feed.UpdateAdminConfig")
	defer span.End()
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid admin feed config: %w", err)
	}
	if err := s.commands.UpsertAdminConfig(ctx, cfg); err != nil {
		return fmt.Errorf("CommandService.UpdateAdminConfig: %w", err)
	}
	s.publish(ctx, feed.FeedConfigChanged{FeedType: cfg.FeedType, OccurredAt: time.Now()})
	return nil
}

func (s *CommandService) BatchUpdateAdminConfigs(ctx context.Context, cfgs []*feed.AdminConfig) error {
	ctx, span := tracing.StartSpan(ctx, "app.feed.BatchUpdateAdminConfigs")
	defer span.End()
	for _, cfg := range cfgs {
		if err := cfg.Validate(); err != nil {
			return fmt.Errorf("invalid admin feed config %q: %w", cfg.FeedType, err)
		}
	}
	if err := s.commands.BatchUpsertAdminConfig(ctx, cfgs); err != nil {
		return fmt.Errorf("CommandService.BatchUpdateAdminConfigs: %w", err)
	}
	now := time.Now()
	for _, cfg := range cfgs {
		s.publish(ctx, feed.FeedConfigChanged{FeedType: cfg.FeedType, OccurredAt: now})
	}
	return nil
}

func (s *CommandService) UpdateProfileConfig(ctx context.Context, cfg *feed.ProfileConfig) error {
	ctx, span := tracing.StartSpan(ctx, "app.feed.UpdateProfileConfig")
	defer span.End()
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid profile feed config: %w", err)
	}
	if err := s.commands.UpsertProfileConfig(ctx, cfg); err != nil {
		return fmt.Errorf("CommandService.UpdateProfileConfig: %w", err)
	}
	s.publish(ctx, feed.FeedConfigChanged{FeedType: cfg.FeedType, ProfileID: cfg.ProfileID, OccurredAt: time.Now()})
	return nil
}

func (s *CommandService) ResetProfileConfig(ctx context.Context, profileID string, feedType feed.Type) error {
	ctx, span := tracing.StartSpan(ctx, "app.feed.ResetProfileConfig")
	defer span.End()
	if err := s.commands.DeleteProfileConfig(ctx, profileID, feedType); err != nil {
		return fmt.Errorf("CommandService.ResetProfileConfig: %w", err)
	}
	s.publish(ctx, feed.FeedConfigRemoved{FeedType: feedType, ProfileID: shared.ProfileID(profileID), OccurredAt: time.Now()})
	return nil
}

func (s *CommandService) EnsureLibraryFeedConfig(ctx context.Context, libraryID shared.LibraryID, name string) error {
	ctx, span := tracing.StartSpan(ctx, "app.feed.EnsureLibraryFeedConfig")
	defer span.End()
	// Read-derive-write, so the whole thing is the critical section, not just
	// the write. This runs once per library creation - the contention is nil.
	s.ensureMu.Lock()
	defer s.ensureMu.Unlock()
	existing, err := s.queries.GetAdminConfig(ctx)
	if err != nil {
		return fmt.Errorf("CommandService.EnsureLibraryFeedConfig: %w", err)
	}
	ft := feed.LibraryType(string(libraryID))
	for _, c := range existing {
		if c.FeedType == ft {
			return nil
		}
	}
	maxOrder := 0
	for _, c := range existing {
		if c.DisplayOrder > maxOrder {
			maxOrder = c.DisplayOrder
		}
	}
	cfg := &feed.AdminConfig{
		FeedType:     ft,
		Enabled:      true,
		DisplayOrder: maxOrder + 1,
		MaxItems:     20,
		Params:       map[string]any{"libraryName": name},
	}
	if err := s.commands.UpsertAdminConfig(ctx, cfg); err != nil {
		return fmt.Errorf("CommandService.EnsureLibraryFeedConfig: %w", err)
	}
	s.publish(ctx, feed.FeedConfigChanged{FeedType: ft, OccurredAt: time.Now()})
	return nil
}

func (s *CommandService) DeleteLibraryFeedConfig(ctx context.Context, libraryID shared.LibraryID) error {
	ctx, span := tracing.StartSpan(ctx, "app.feed.DeleteLibraryFeedConfig")
	defer span.End()
	ft := feed.LibraryType(string(libraryID))
	if err := s.commands.DeleteAdminConfigByType(ctx, ft); err != nil {
		return fmt.Errorf("CommandService.DeleteLibraryFeedConfig: %w", err)
	}
	s.publish(ctx, feed.FeedConfigRemoved{FeedType: ft, OccurredAt: time.Now()})
	return nil
}

// DeletePluginFeedConfig removes the plugin:<id> row from the admin config and
// from every profile's config once the plugin is gone.
func (s *CommandService) DeletePluginFeedConfig(ctx context.Context, pluginID string) error {
	ctx, span := tracing.StartSpan(ctx, "app.feed.DeletePluginFeedConfig")
	defer span.End()
	ft := feed.PluginType(pluginID)
	if err := s.commands.DeleteAdminConfigByType(ctx, ft); err != nil {
		return fmt.Errorf("CommandService.DeletePluginFeedConfig: %w", err)
	}
	s.publish(ctx, feed.FeedConfigRemoved{FeedType: ft, OccurredAt: time.Now()})
	return nil
}
