package user

import (
	"errors"

	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/gravity-zero/evey/internal/domain/user"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserDisabled       = errors.New("user is disabled")
	ErrNotAdmin           = errors.New("admin required")
	ErrLastAdmin          = errors.New("cannot remove last admin")
	ErrSetupDone          = errors.New("setup already completed")
	ErrInviteInvalid      = errors.New("invalid or expired invite")
	ErrPasswordTooShort   = errors.New("password must be at least 8 characters")
)

// Service is a backward-compat alias for CommandService.
type Service = CommandService

// NewService creates a CommandService. Kept for compatibility.
func NewService(
	users user.UserRepository,
	profiles user.ProfileRepository,
	refreshTkns user.RefreshTokenRepository,
	apiKeys user.APIKeyRepository,
	invites user.InviteLinkRepository,
	quickConnect user.QuickConnectRepository,
	roles user.RoleRepository,
	userRoles user.UserRoleRepository,
	hasher user.PasswordHasher,
	tokens user.TokenGenerator,
	events shared.EventBus,
) *CommandService {
	return NewCommandService(users, profiles, refreshTkns, apiKeys, invites, quickConnect, roles, userRoles, hasher, tokens, events)
}
