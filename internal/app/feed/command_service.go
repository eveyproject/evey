package feed

import (
	"context"
	"fmt"

	"github.com/gravity-zero/evey/internal/domain/feed"
)

type CommandService struct {
	commands feed.ConfigCommandRepo
}

func NewCommandService(commands feed.ConfigCommandRepo) *CommandService {
	return &CommandService{commands: commands}
}

func (s *CommandService) UpdateAdminConfig(ctx context.Context, cfg *feed.AdminConfig) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid admin feed config: %w", err)
	}
	return s.commands.UpsertAdminConfig(ctx, cfg)
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
