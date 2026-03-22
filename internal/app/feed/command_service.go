package feed

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/gravity-zero/evey/internal/domain/feed"
	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/shared"
)

type CommandService struct {
	commands feed.ConfigCommandRepo
	queries  feed.ConfigQueryRepo
}

func NewCommandService(commands feed.ConfigCommandRepo, queries feed.ConfigQueryRepo) *CommandService {
	return &CommandService{commands: commands, queries: queries}
}

func (s *CommandService) WithEventBus(bus shared.EventBus) *CommandService {
	bus.Subscribe("library.created", func(ctx context.Context, ev shared.Event) {
		e, ok := ev.(library.LibraryCreated)
		if !ok {
			return
		}
		if err := s.EnsureLibraryFeedConfig(ctx, e.LibraryID, e.Name); err != nil {
			slog.Warn("auto-create library feed config", slog.String("library", e.LibraryID), slog.Any("err", err))
		}
	})
	bus.Subscribe("library.deleted", func(ctx context.Context, ev shared.Event) {
		e, ok := ev.(library.LibraryDeleted)
		if !ok {
			return
		}
		if err := s.DeleteLibraryFeedConfig(ctx, e.LibraryID); err != nil {
			slog.Warn("auto-delete library feed config", slog.String("library", e.LibraryID), slog.Any("err", err))
		}
	})
	return s
}

func (s *CommandService) UpdateAdminConfig(ctx context.Context, cfg *feed.AdminConfig) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid admin feed config: %w", err)
	}
	return s.commands.UpsertAdminConfig(ctx, cfg)
}

func (s *CommandService) BatchUpdateAdminConfigs(ctx context.Context, cfgs []*feed.AdminConfig) error {
	for _, cfg := range cfgs {
		if err := cfg.Validate(); err != nil {
			return fmt.Errorf("invalid admin feed config %q: %w", cfg.FeedType, err)
		}
	}
	return s.commands.BatchUpsertAdminConfig(ctx, cfgs)
}

func (s *CommandService) UpdateProfileConfig(ctx context.Context, cfg *feed.ProfileConfig) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid profile feed config: %w", err)
	}
	return s.commands.UpsertProfileConfig(ctx, cfg)
}

func (s *CommandService) ResetProfileConfig(ctx context.Context, profileID string, feedType feed.Type) error {
	return s.commands.DeleteProfileConfig(ctx, profileID, feedType)
}

func (s *CommandService) EnsureLibraryFeedConfig(ctx context.Context, libraryID, name string) error {
	existing, err := s.queries.GetAdminConfig(ctx)
	if err != nil {
		return err
	}
	ft := feed.LibraryType(libraryID)
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
	return s.commands.UpsertAdminConfig(ctx, &feed.AdminConfig{
		FeedType:     ft,
		Enabled:      true,
		DisplayOrder: maxOrder + 1,
		MaxItems:     20,
		Params:       map[string]any{"libraryName": name},
	})
}

func (s *CommandService) DeleteLibraryFeedConfig(ctx context.Context, libraryID string) error {
	return s.commands.DeleteAdminConfigByType(ctx, feed.LibraryType(libraryID))
}
