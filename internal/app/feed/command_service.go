package feed

import (
	"context"

	"github.com/gravity-zero/evey/internal/domain/feed"
)

type CommandService struct {
	commands feed.ConfigCommandRepo
}

func NewCommandService(commands feed.ConfigCommandRepo) *CommandService {
	return &CommandService{commands: commands}
}

func (s *CommandService) UpdateAdminConfig(ctx context.Context, cfg *feed.AdminConfig) error {
	return s.commands.UpsertAdminConfig(ctx, cfg)
}

func (s *CommandService) UpdateProfileConfig(ctx context.Context, cfg *feed.ProfileConfig) error {
	return s.commands.UpsertProfileConfig(ctx, cfg)
}

func (s *CommandService) ResetProfileConfig(ctx context.Context, profileID string, feedType feed.Type) error {
	return s.commands.DeleteProfileConfig(ctx, profileID, feedType)
}
