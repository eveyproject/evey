package user

import (
	"context"
	"fmt"

	"github.com/gravity-zero/evey/internal/domain/user"
)

type QueryService struct {
	users        user.UserRepository
	profiles     user.ProfileRepository
	apiKeys      user.APIKeyRepository
	invites      user.InviteLinkRepository
	quickConnect user.QuickConnectRepository
}

func NewQueryService(
	users user.UserRepository,
	profiles user.ProfileRepository,
	apiKeys user.APIKeyRepository,
	invites user.InviteLinkRepository,
	quickConnect user.QuickConnectRepository,
) *QueryService {
	return &QueryService{
		users:        users,
		profiles:     profiles,
		apiKeys:      apiKeys,
		invites:      invites,
		quickConnect: quickConnect,
	}
}

func (s *QueryService) CountAdmins(ctx context.Context) (int, error) {
	return s.users.CountAdmins(ctx)
}

func (s *QueryService) ListUsers(ctx context.Context) ([]*user.User, error) {
	return s.users.List(ctx)
}

func (s *QueryService) ListProfiles(ctx context.Context, userID string) ([]*user.Profile, error) {
	return s.profiles.GetByUserID(ctx, userID)
}

func (s *QueryService) ListInviteLinks(ctx context.Context, creatorID string) ([]*user.InviteLink, error) {
	return s.invites.ListByCreator(ctx, creatorID)
}

func (s *QueryService) CheckQuickConnect(ctx context.Context, code string) (*user.QuickConnectRequest, error) {
	return s.quickConnect.GetByCode(ctx, code)
}

func (s *QueryService) ListAPIKeys(ctx context.Context, userID string) ([]*user.APIKeyModel, error) {
	return s.apiKeys.ListByUserID(ctx, userID)
}

func (s *QueryService) GetUserByID(ctx context.Context, id string) (*user.User, error) {
	return s.users.GetByID(ctx, id)
}

func (s *QueryService) GetUserByUsername(ctx context.Context, username string) (*user.User, error) {
	return s.users.GetByUsername(ctx, username)
}

func (s *QueryService) GetProfileByID(ctx context.Context, profileID string) (*user.Profile, error) {
	return s.profiles.GetByID(ctx, profileID)
}

func (s *QueryService) GetUserBitrateLimit(ctx context.Context, userID string) (int, error) {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("QueryService.GetUserBitrateLimit: %w", err)
	}
	return u.MaxBitrateKbps, nil
}
