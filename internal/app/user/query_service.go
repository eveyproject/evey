package user

import (
	"context"

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
