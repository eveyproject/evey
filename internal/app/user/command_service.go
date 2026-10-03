package user

import (
	"strings"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
)

// command_service.go holds the CommandService type, its wiring (options +
// constructor) and the constants shared across its responsibilities. The
// responsibilities themselves live in cohesive sibling files:
//   - setup.go                 first-run admin bootstrap
//   - auth.go                  login / impersonation / refresh-token rotation
//   - password_reset.go        email-based password recovery
//   - cleanup.go               background token/login-attempt sweep loops
//   - login_attempt_limiter.go the default in-memory rate limiter

const (
	loginMaxAttempts   = 5 // per (client IP, user)
	loginAttemptWindow = 15 * time.Minute
	// loginUserMaxAttempts caps one account across all IPs: high enough that a
	// stranger needs many addresses to lock it, low enough to stop spraying.
	loginUserMaxAttempts = 50

	// defaultMinPasswordLength is enforced by both Setup and ResetPassword,
	// unless WithMinPasswordLength lowers it at composition (the demo).
	defaultMinPasswordLength = 8
)

// CommandService handles auth, setup and token management.
type CommandService struct {
	users             user.UserRepository
	profiles          user.ProfileRepository
	refreshTkns       user.RefreshTokenRepository
	resetTokens       user.PasswordResetTokenRepository
	roles             user.RoleRepository
	userRoles         user.UserRoleRepository
	hasher            user.PasswordHasher
	tokens            user.TokenGenerator
	events            shared.EventBus
	quickConnect      user.QuickConnectRepository
	serverCfg         shared.ServerConfigRepository
	smtpConfigured    func() bool // read per request: SMTP is live-editable
	refreshTokenTTL   time.Duration
	loginAttempts     user.LoginAttemptRepository
	loginMaxFails     int
	loginUserMaxFails int
	loginWindow       time.Duration
	// lockExempt is a username the failed-login counters never lock (the
	// extended demo's guest account: a mistyped password on a TV
	// remote must not close the only door). Empty: nobody is exempt.
	lockExempt   string
	setupMu      sync.Mutex
	setupDone    bool
	uow          shared.TransactionalUnit
	tokenRevoker TokenRevoker // optional: bumps token_epoch on security events
	// minPasswordLength is the floor Setup and ResetPassword enforce.
	minPasswordLength int
}

func NewCommandService(
	users user.UserRepository,
	profiles user.ProfileRepository,
	refreshTkns user.RefreshTokenRepository,
	roles user.RoleRepository,
	userRoles user.UserRoleRepository,
	hasher user.PasswordHasher,
	tokens user.TokenGenerator,
	events shared.EventBus,
	refreshTokenTTL time.Duration,
	quickConnect user.QuickConnectRepository,
	opts ...CommandServiceOption,
) *CommandService {
	s := &CommandService{
		users:             users,
		profiles:          profiles,
		refreshTkns:       refreshTkns,
		roles:             roles,
		userRoles:         userRoles,
		hasher:            hasher,
		tokens:            tokens,
		events:            events,
		refreshTokenTTL:   refreshTokenTTL,
		quickConnect:      quickConnect,
		loginAttempts:     newLoginAttemptLimiter(loginMaxAttempts, loginAttemptWindow),
		loginMaxFails:     loginMaxAttempts,
		loginUserMaxFails: loginUserMaxAttempts,
		loginWindow:       loginAttemptWindow,

		minPasswordLength: defaultMinPasswordLength,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

type CommandServiceOption func(*CommandService)

// WithMinPasswordLength lowers the password floor Setup and ResetPassword
// enforce. Composition-level on purpose: the one caller is the demo instance,
// whose published credentials are shorter than the production floor, and the
// floor is a parameter of the service rather than a mode the service knows
// about. A value below 1 is ignored.
func WithMinPasswordLength(n int) CommandServiceOption {
	return func(s *CommandService) {
		if n >= 1 {
			s.minPasswordLength = n
		}
	}
}

func WithPasswordReset(resetTokens user.PasswordResetTokenRepository, serverCfg shared.ServerConfigRepository, smtpConfigured bool) CommandServiceOption {
	return func(s *CommandService) {
		s.resetTokens = resetTokens
		s.serverCfg = serverCfg
		s.smtpConfigured = func() bool { return smtpConfigured }
	}
}

// WithLiveSMTPConfigured replaces WithPasswordReset's boot-time SMTP flag with
// a function read on every request, so SMTP set up from the panel later turns
// password recovery on without a restart.
func WithLiveSMTPConfigured(fn func() bool) CommandServiceOption {
	return func(s *CommandService) { s.smtpConfigured = fn }
}

// WithUnitOfWork wires a transactional unit so multi-step writes (e.g.
// password reset: user update + token mark-used) commit/rollback atomically.
func WithUnitOfWork(uow shared.TransactionalUnit) CommandServiceOption {
	return func(s *CommandService) { s.uow = uow }
}

// WithLoginAttemptRepository replaces the default in-memory rate limiter with
// a persistent one (e.g. postgres) so lockout state survives restarts and is
// shared across instances.
func WithLoginAttemptRepository(repo user.LoginAttemptRepository) CommandServiceOption {
	return func(s *CommandService) { s.loginAttempts = repo }
}

// CommandWithTokenRevoker wires access-token revocation (token_epoch bump)
// into the security-event paths owned by CommandService (password reset).
// CommandWithLoginLockExempt makes username immune to the failed-login lock.
// Its password is long and random, which is what stands in for the lock.
func CommandWithLoginLockExempt(username string) CommandServiceOption {
	return func(s *CommandService) { s.lockExempt = strings.TrimSpace(username) }
}

func CommandWithTokenRevoker(rev TokenRevoker) CommandServiceOption {
	return func(s *CommandService) { s.tokenRevoker = rev }
}
