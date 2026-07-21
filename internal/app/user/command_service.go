package user

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/id"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

const (
	loginMaxAttempts   = 5
	loginAttemptWindow = 15 * time.Minute

	minPasswordLength      = 8
	loginLimiterMaxEntries = 10000
	defaultCleanupInterval = 1 * time.Hour
	resetTokenBytes        = 32
)

// safeTokenPrefix returns the first 8 chars of a token (safe for logs).
func safeTokenPrefix(token string) string {
	if len(token) < 8 {
		return "<short>"
	}
	return token[:8] + "…"
}

// CommandService handles auth, setup and token management.
type CommandService struct {
	users           user.UserRepository
	profiles        user.ProfileRepository
	refreshTkns     user.RefreshTokenRepository
	resetTokens     user.PasswordResetTokenRepository
	roles           user.RoleRepository
	userRoles       user.UserRoleRepository
	hasher          user.PasswordHasher
	tokens          user.TokenGenerator
	events          shared.EventBus
	quickConnect    user.QuickConnectRepository
	serverCfg       shared.ServerConfigRepository
	smtpConfigured  bool
	refreshTokenTTL time.Duration
	loginAttempts   user.LoginAttemptRepository
	loginMaxFails   int
	loginWindow     time.Duration
	setupMu         sync.Mutex
	setupDone       bool
	uow             shared.TransactionalUnit
	tokenRevoker    TokenRevoker // optional: bumps token_epoch on security events
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
		users:           users,
		profiles:        profiles,
		refreshTkns:     refreshTkns,
		roles:           roles,
		userRoles:       userRoles,
		hasher:          hasher,
		tokens:          tokens,
		events:          events,
		refreshTokenTTL: refreshTokenTTL,
		quickConnect:    quickConnect,
		loginAttempts:   newLoginAttemptLimiter(loginMaxAttempts, loginAttemptWindow),
		loginMaxFails:   loginMaxAttempts,
		loginWindow:     loginAttemptWindow,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

type CommandServiceOption func(*CommandService)

func WithPasswordReset(resetTokens user.PasswordResetTokenRepository, serverCfg shared.ServerConfigRepository, smtpConfigured bool) CommandServiceOption {
	return func(s *CommandService) {
		s.resetTokens = resetTokens
		s.serverCfg = serverCfg
		s.smtpConfigured = smtpConfigured
	}
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
func CommandWithTokenRevoker(rev TokenRevoker) CommandServiceOption {
	return func(s *CommandService) { s.tokenRevoker = rev }
}

func (s *CommandService) InitSetupState(ctx context.Context) {
	count, err := s.users.CountAdmins(ctx)
	if err == nil && count > 0 {
		s.setupDone = true
	}
}

// SetupOption configures optional inputs for the initial admin Setup.
type SetupOption func(*setupOptions)

type setupOptions struct {
	locale string
	region string
}

// WithSetupLocale seeds the admin default profile with the language the user
// picked in the setup wizard, so the JWT locale claim and subsequent sessions
// stay in that language instead of falling back to the DB column default.
func WithSetupLocale(locale string) SetupOption {
	return func(o *setupOptions) { o.locale = locale }
}

// WithSetupRegion seeds the admin default profile with an ISO-3166 region.
// Empty string is ignored (region stays nil → server fallback at runtime).
func WithSetupRegion(region string) SetupOption {
	return func(o *setupOptions) { o.region = region }
}

func (s *CommandService) Setup(ctx context.Context, username, password, email string, opts ...SetupOption) (*user.User, error) {
	ctx, span := tracing.StartSpan(ctx, "app.user.Setup")
	defer span.End()
	var cfg setupOptions
	for _, fn := range opts {
		fn(&cfg)
	}
	s.setupMu.Lock()
	defer s.setupMu.Unlock()
	if s.setupDone {
		return nil, ErrSetupDone
	}
	if username == "" {
		return nil, errors.New("username is required")
	}
	if len(password) < minPasswordLength {
		return nil, ErrPasswordTooShort
	}
	admins, err := s.users.CountAdmins(ctx)
	if err != nil {
		return nil, fmt.Errorf("CommandService.Setup: %w", err)
	}
	if admins > 0 {
		s.setupDone = true
		return nil, ErrSetupDone
	}

	hash, err := s.hasher.Hash(password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	u, err := user.NewUser(username, email, hash)
	if err != nil {
		return nil, fmt.Errorf("CommandService.Setup: %w", err)
	}
	u.IsAdmin = true

	profileOpts := []user.ProfileOption{
		user.WithProfileDefault(true),
		user.WithProfileLocale(cfg.locale),
	}
	if cfg.region != "" {
		profileOpts = append(profileOpts, user.WithProfileRegion(cfg.region))
	}
	p, err := user.NewProfile(shared.ProfileID(id.New("prf-")), u.ID, u.Username, profileOpts...)
	if err != nil {
		return nil, fmt.Errorf("CommandService.Setup: build default profile: %w", err)
	}

	// The admin user and its default profile must be created atomically: a
	// profile-insert failure after the user row commits would leave the sole
	// admin with NO default profile AND lock Setup forever (the retry sees
	// CountAdmins>0 → ErrSetupDone), with no code path that heals a missing
	// default profile. Wrap both in one tx, mirroring UserService.CreateUser.
	apply := func(ctx context.Context) error {
		if err := s.users.Create(ctx, u); err != nil {
			return fmt.Errorf("create admin user: %w", err)
		}
		if err := s.profiles.Create(ctx, p); err != nil {
			return fmt.Errorf("create default profile: %w", err)
		}
		return nil
	}
	var txErr error
	if s.uow != nil {
		txErr = s.uow.Do(ctx, apply)
	} else {
		txErr = apply(ctx)
	}
	if txErr != nil {
		return nil, fmt.Errorf("CommandService.Setup: %w", txErr)
	}

	// Events are published after the commit so a rolled-back Setup never emits
	// a phantom UserCreated/ProfileCreated.
	evtSetupCreated := user.UserCreated{UserID: u.ID, Username: u.Username, IsAdmin: true, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtSetupCreated); err != nil {
		slog.Warn("publish event", slog.String("event", evtSetupCreated.EventName()), slog.Any("err", err))
	}
	evtProfileCreated := user.ProfileCreated{ProfileID: p.ID, UserID: p.UserID, Name: p.Name, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtProfileCreated); err != nil {
		slog.Warn("publish event", slog.String("event", evtProfileCreated.EventName()), slog.Any("err", err))
	}

	s.setupDone = true
	return u, nil
}

// dummyLoginHash is a valid cost-12 bcrypt hash of a throwaway string. It is
// verified against the submitted password when the username does NOT exist, so
// a failed login costs the same ~100ms whether or not the account is real -
// closing the timing oracle that would otherwise enumerate usernames. It is
// never a usable credential (no account stores it).
const dummyLoginHash = "$2a$12$CCENmDmu1fye2Bn2DUS7G.4pIkth0OyzzFCkIQvblCV9aye/etkwO"

func (s *CommandService) Login(ctx context.Context, username, password string, ttlOverride ...time.Duration) (accessToken, refreshToken string, err error) {
	ctx, span := tracing.StartSpan(ctx, "app.user.Login")
	defer span.End()
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	locked, lockErr := s.loginAttempts.IsLocked(ctx, username)
	if lockErr != nil {
		slog.Warn("login attempt IsLocked", slog.String("username", username), slog.Any("err", lockErr))
	}
	if locked {
		return "", "", ErrAccountLocked
	}

	u, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		// Constant-time defense against username enumeration: a missing user must
		// cost the same as a wrong password. Without this, the bcrypt verify below
		// runs ONLY for existing users, so the ~100ms (cost-12) delta leaks which
		// usernames exist. Burn an equivalent bcrypt verify against a fixed hash.
		_, _ = s.hasher.Verify(password, dummyLoginHash) //nolint:errcheck // constant-time burn; result intentionally discarded
		if recErr := s.loginAttempts.Record(ctx, username, s.loginMaxFails, s.loginWindow); recErr != nil {
			slog.Warn("login attempt Record", slog.String("username", username), slog.Any("err", recErr))
		}
		// Per-attempt log (username only, never the password) so a
		// credential-stuffing run is searchable, not just inferable from the
		// rate-limit store's aggregate.
		slog.Warn("login failed", slog.String("username", username), slog.String("reason", "unknown_user"))
		return "", "", ErrInvalidCredentials
	}
	// Verify BEFORE the CanLogin gate so a disabled account costs the same bcrypt
	// as an enabled account with a wrong password. Returning ErrUserDisabled
	// without running the hash lets timing separate "exists & disabled" from the
	// other states (the error is flattened to a generic message above the HTTP
	// boundary, so timing is the only remaining oracle).
	ok, err := s.hasher.Verify(password, u.PasswordHash)
	if !u.CanLogin() {
		return "", "", ErrUserDisabled
	}
	if err != nil || !ok {
		if recErr := s.loginAttempts.Record(ctx, username, s.loginMaxFails, s.loginWindow); recErr != nil {
			slog.Warn("login attempt Record", slog.String("username", username), slog.Any("err", recErr))
		}
		slog.Warn("login failed", slog.String("username", username), slog.String("reason", "bad_password"))
		return "", "", ErrInvalidCredentials
	}
	if resetErr := s.loginAttempts.Reset(ctx, username); resetErr != nil {
		slog.Warn("login attempt Reset", slog.String("username", username), slog.Any("err", resetErr))
	}

	perms := s.collectPermissions(ctx, u.ID)

	var defaultProfileID string
	// Fail CLOSED on a lookup error. An empty ProfileID is read as "no parental
	// restriction" by every gate in the server, so continuing past this fault
	// would issue an unrestricted token. Same reasoning as RefreshToken.
	profs, pErr := s.profiles.GetByUserID(ctx, u.ID)
	if pErr != nil {
		slog.Warn("profile lookup failed, refusing rather than issuing a profile-less token",
			slog.String("userID", string(u.ID)), slog.Any("err", pErr))
		return "", "", fmt.Errorf("load profiles: %w", pErr)
	}
	for _, p := range profs {
		if p.IsDefault {
			defaultProfileID = string(p.ID)
			break
		}
	}
	if defaultProfileID == "" && len(profs) > 0 {
		defaultProfileID = string(profs[0].ID)
	}
	var profileLocale string
	if defaultProfileID != "" {
		if prof, pErr := s.profiles.GetByID(ctx, shared.ProfileID(defaultProfileID)); pErr == nil && prof != nil {
			profileLocale = prof.Locale
		}
	}

	accessToken, err = s.tokens.GenerateAccessToken(user.Claims{
		UserID:      string(u.ID),
		Username:    u.Username,
		IsAdmin:     u.IsAdmin,
		ProfileID:   defaultProfileID,
		Locale:      profileLocale,
		Permissions: perms,
	})
	if err != nil {
		return "", "", fmt.Errorf("generate access token: %w", err)
	}

	refreshToken, err = s.generateAndStoreRefreshToken(ctx, string(u.ID), defaultProfileID, ttlOverride...)
	if err != nil {
		return "", "", fmt.Errorf("CommandService.Login: %w", err)
	}

	evtLogin := user.LoginSucceeded{UserID: u.ID, AuthMethod: "local", OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtLogin); err != nil {
		slog.Warn("publish event", slog.String("event", evtLogin.EventName()), slog.Any("err", err))
	}
	if err := s.users.TouchLastLogin(ctx, u.ID, evtLogin.OccurredAt); err != nil {
		slog.Warn("touch last_login", slog.String("user_id", string(u.ID)), slog.Any("err", err))
	}
	return accessToken, refreshToken, nil
}

func (s *CommandService) GenerateTokensForUser(ctx context.Context, userID string, ttlOverride ...time.Duration) (accessToken, refreshToken string, err error) {
	ctx, span := tracing.StartSpan(ctx, "app.user.GenerateTokensForUser")
	defer span.End()
	u, err := s.users.GetByID(ctx, shared.UserID(userID))
	if err != nil {
		return "", "", fmt.Errorf("user not found: %w", err)
	}
	if !u.CanLogin() {
		return "", "", ErrUserDisabled
	}

	perms := s.collectPermissions(ctx, u.ID)

	var defaultProfileID string
	// Fail CLOSED on a lookup error. An empty ProfileID is read as "no parental
	// restriction" by every gate in the server, so continuing past this fault
	// would issue an unrestricted token. Same reasoning as RefreshToken.
	profs, pErr := s.profiles.GetByUserID(ctx, u.ID)
	if pErr != nil {
		slog.Warn("profile lookup failed, refusing rather than issuing a profile-less token",
			slog.String("userID", string(u.ID)), slog.Any("err", pErr))
		return "", "", fmt.Errorf("load profiles: %w", pErr)
	}
	for _, p := range profs {
		if p.IsDefault {
			defaultProfileID = string(p.ID)
			break
		}
	}
	if defaultProfileID == "" && len(profs) > 0 {
		defaultProfileID = string(profs[0].ID)
	}
	var profileLocale string
	if defaultProfileID != "" {
		if prof, pErr := s.profiles.GetByID(ctx, shared.ProfileID(defaultProfileID)); pErr == nil && prof != nil {
			profileLocale = prof.Locale
		}
	}

	accessToken, err = s.tokens.GenerateAccessToken(user.Claims{
		UserID:      string(u.ID),
		Username:    u.Username,
		IsAdmin:     u.IsAdmin,
		ProfileID:   defaultProfileID,
		Locale:      profileLocale,
		Permissions: perms,
	})
	if err != nil {
		return "", "", fmt.Errorf("generate access token: %w", err)
	}

	refreshToken, err = s.generateAndStoreRefreshToken(ctx, string(u.ID), defaultProfileID, ttlOverride...)
	if err != nil {
		return "", "", fmt.Errorf("CommandService.GenerateTokensForUser: %w", err)
	}
	return accessToken, refreshToken, nil
}

// Impersonate mints a SHORT-LIVED access token that lets an admin act as the
// target user/profile ("connect as"). The returned token's claims ARE the
// target's (full power as them) plus an ImpersonatedBy stamp recording the admin
// for audit. No refresh token is issued: the session is bounded by the access
// TTL and the admin re-impersonates to extend it, so impersonation can't
// silently persist. Caller (handler) is responsible for the admin-only +
// not-an-API-key gate and the audit-log entry.
func (s *CommandService) Impersonate(ctx context.Context, adminUserID, targetUserID, targetProfileID string) (string, error) {
	ctx, span := tracing.StartSpan(ctx, "app.user.Impersonate")
	defer span.End()
	if adminUserID == "" {
		return "", ErrInvalidCredentials
	}
	u, err := s.users.GetByID(ctx, shared.UserID(targetUserID))
	if err != nil {
		return "", ErrUserNotFound
	}
	if !u.CanLogin() {
		return "", ErrUserDisabled
	}
	profs, pErr := s.profiles.GetByUserID(ctx, u.ID)
	if pErr != nil {
		return "", fmt.Errorf("CommandService.Impersonate: load profiles: %w", pErr)
	}
	// resolveActiveProfileID rejects a profile that does not belong to the target
	// (and picks the default when targetProfileID is empty).
	activeProfileID, ok := resolveActiveProfileID(profs, targetProfileID)
	if !ok {
		return "", ErrProfileNotFound
	}
	perms := s.collectPermissions(ctx, u.ID)
	accessToken, err := s.tokens.GenerateAccessToken(user.Claims{
		UserID:         string(u.ID),
		Username:       u.Username,
		IsAdmin:        u.IsAdmin,
		ProfileID:      activeProfileID,
		Locale:         s.lookupProfileLocale(ctx, activeProfileID),
		Permissions:    perms,
		ImpersonatedBy: adminUserID,
	})
	if err != nil {
		return "", fmt.Errorf("CommandService.Impersonate: generate token: %w", err)
	}
	return accessToken, nil
}

// RefreshToken rotates the refresh token, returning a new access+refresh pair.
// requestedProfileID (optional) activates a specific profile on the new access
// token; must belong to the refreshing user or the refresh is rejected.
func (s *CommandService) RefreshToken(ctx context.Context, token, requestedProfileID, pinToken string, ttlOverride ...time.Duration) (string, string, error) {
	ctx, span := tracing.StartSpan(ctx, "app.user.RefreshToken")
	defer span.End()
	rt, graced, err := s.validateRefreshToken(ctx, token)
	if err != nil {
		return "", "", err
	}

	u, err := s.users.GetByID(ctx, rt.UserID)
	if err != nil {
		return "", "", fmt.Errorf("CommandService.RefreshToken: %w", err)
	}
	if !u.Enabled {
		return "", "", ErrUserDisabled
	}

	perms := s.collectPermissions(ctx, u.ID)

	profs, pErr := s.profiles.GetByUserID(ctx, u.ID)
	if pErr != nil {
		// Fail CLOSED. Continuing with a nil slice makes resolveActiveProfileID
		// return ("", true) - a token carrying NO profile - and an empty
		// ProfileID is read as "no parental restriction" by every gate in the
		// server (the HTTP checker and the gRPC filter resolver both
		// short-circuit on it). So swallowing this turned a transient DB fault
		// into a silent, complete parental bypass for the whole token TTL, and
		// a permanently unscannable profile row into a permanent one.
		//
		// Refusing logs the user out, which is recoverable. Handing out an
		// unrestricted token is not.
		slog.Warn("refresh: profile lookup failed, refusing rather than dropping the profile binding",
			slog.String("userID", string(u.ID)), slog.Any("err", pErr))
		return "", "", fmt.Errorf("refresh: load profiles: %w", pErr)
	}
	// A silent refresh (empty requestedProfileID) must PRESERVE the profile this
	// token was issued for - not silently fall back to the default profile,
	// which let a kids profile escape parental control on token expiry. An
	// explicit requestedProfileID (a profile switch) still wins.
	effectiveRequested := requestedProfileID
	if effectiveRequested == "" && rt.ProfileID != nil {
		effectiveRequested = string(*rt.ProfileID)
	}
	activeProfileID, ok := resolveActiveProfileID(profs, effectiveRequested)
	if !ok {
		if requestedProfileID != "" {
			// Caller explicitly asked for a profile that doesn't belong to them - reject.
			return "", "", ErrInvalidCredentials
		}
		// The bound profile no longer exists (deleted) - fall back to default.
		activeProfileID, _ = resolveActiveProfileID(profs, "")
	}

	// Parental PIN gate: switching OUT of a parental-restricted profile (a kids
	// profile, or a non-kids one whose owner explicitly enabled parental
	// controls with a PIN) INTO an unrestricted non-kids one requires a
	// server-verified PIN proof for the target profile. Enforced here (not just
	// in the UI) so a direct API call can't bypass parental control: without it
	// the content filter such a profile carries is defeated by one refresh call.
	if rt.ProfileID != nil && string(*rt.ProfileID) != activeProfileID {
		old := findProfileByID(profs, string(*rt.ProfileID))
		target := findProfileByID(profs, activeProfileID)
		if old != nil && target != nil && old.IsParentalRestricted() {
			// Which PINs authorise this exit. The target's PIN is the historical
			// proof (a kid cannot know the adult profile's PIN). A gated non-kids
			// profile additionally accepts its OWN PIN: its target may legitimately
			// have no PIN at all, and without this the whole gate would be a no-op
			// for exactly the setups it protects.
			var authorisers []string
			if !target.IsKids && target.HasPIN() {
				authorisers = append(authorisers, string(target.ID))
			}
			if !old.IsKids && old.HasPIN() && !target.IsParentalRestricted() {
				authorisers = append(authorisers, string(old.ID))
			}
			if len(authorisers) > 0 {
				provenID, perr := s.tokens.ParsePINToken(pinToken)
				if perr != nil || !slices.Contains(authorisers, provenID) {
					// Deliberately BEFORE the token is consumed: the client
					// fetches a PIN proof and retries with this same refresh
					// token.
					return "", "", ErrPINRequired
				}
			}
		}
	}

	// The access token is pure JWT signing - keep it outside the transaction.
	accessToken, err := s.tokens.GenerateAccessToken(user.Claims{
		UserID:      string(u.ID),
		Username:    u.Username,
		IsAdmin:     u.IsAdmin,
		ProfileID:   activeProfileID,
		Locale:      s.lookupProfileLocale(ctx, activeProfileID),
		Permissions: perms,
	})
	if err != nil {
		return "", "", fmt.Errorf("CommandService.RefreshToken: %w", err)
	}

	// Every rejection above leaves the refresh token usable; from here the
	// rotation is committed - ATOMICALLY when a unit of work is wired: claim
	// and replacement insert both land or neither does. A crash mid-rotation
	// can then never leave a claimed-but-unreplaced row behind, which the
	// client's still-stored token would replay into a logout.
	//
	// The claimed row is deliberately KEPT (not deleted): its used_at stamp is
	// what lets a multi-tab race loser presenting the same token within the
	// grace window be recognised and issued a sibling session instead of a
	// 401 that kills its tab. The rows are swept by DeleteExpired once the
	// grace has long passed.
	var newRefresh string
	rotate := func(ctx context.Context) error {
		if !graced {
			if markErr := s.markRefreshTokenUsed(ctx, rt); markErr != nil {
				if !errors.Is(markErr, ErrInvalidCredentials) {
					return markErr
				}
				// Lost the claim to a concurrent rotation that committed
				// between our validate and this claim - milliseconds ago,
				// squarely within the grace. Fall through and issue the
				// sibling session, exactly like the graced path.
			}
		}
		var genErr error
		newRefresh, genErr = s.generateAndStoreRefreshToken(ctx, string(u.ID), activeProfileID, ttlOverride...)
		if genErr != nil {
			return fmt.Errorf("CommandService.RefreshToken: %w", genErr)
		}
		return nil
	}
	var rotErr error
	if s.uow != nil {
		rotErr = s.uow.Do(ctx, rotate)
	} else {
		rotErr = rotate(ctx)
	}
	if rotErr != nil {
		return "", "", rotErr
	}

	return accessToken, newRefresh, nil
}

// refreshRotationGrace is how long a just-rotated refresh token remains
// exchangeable for a SIBLING session. Multiple tabs share one HttpOnly
// refresh cookie; when they refresh concurrently, the losers present a token
// that was rotated milliseconds earlier - rejecting them 401-kills their tab
// (every call fails until a manual reload). Within the grace each loser gets
// its own fresh token pair instead. Kept short: a stolen token replayed
// PAST the grace is still rejected, and the earlier design already dropped
// replay-triggered session purges as daily false positives (see
// validateRefreshToken) - this closes the UX hole that decision left open.
const refreshRotationGrace = 30 * time.Second

// validateRefreshToken performs the hash-lookup + reuse/expiry gating (it does
// NOT consume the token; see markRefreshTokenUsed). Any failure path returns
// ErrInvalidCredentials. The bool reports a GRACED token: one already rotated
// within refreshRotationGrace - a multi-tab race loser, entitled to a sibling
// session (the caller must then skip the claim). A rotation older than the
// grace is a genuine replay and is rejected WITHOUT purging the user's other
// sessions: every purge this branch used to fire was a multi-tab race loser
// or an interrupted rotation, and it logged the whole account out of every
// device (préprod: daily false positives).
func (s *CommandService) validateRefreshToken(ctx context.Context, token string) (*user.RefreshToken, bool, error) {
	h := sha256.Sum256([]byte(token))
	hash := hex.EncodeToString(h[:])

	rt, err := s.refreshTkns.GetByTokenHash(ctx, hash)
	if err != nil {
		slog.Info("refresh rejected: token not found", slog.String("tokenPrefix", safeTokenPrefix(token)))
		return nil, false, ErrInvalidCredentials
	}

	if rt.IsExpired() {
		slog.Info("refresh rejected: token expired",
			slog.String("userID", string(rt.UserID)),
			slog.String("tokenID", rt.ID),
			slog.Time("expiredAt", rt.ExpiresAt))
		if delErr := s.refreshTkns.DeleteByID(ctx, rt.ID); delErr != nil {
			slog.Warn("delete refresh token failed", slog.String("tokenID", rt.ID), slog.Any("err", delErr))
		}
		return nil, false, ErrInvalidCredentials
	}

	if rt.Used {
		// nil UsedAt (legacy row claimed before the column existed) fails
		// closed: no timestamp, no grace.
		if rt.UsedAt != nil && time.Since(*rt.UsedAt) <= refreshRotationGrace {
			slog.Info("refresh: rotated token within grace, issuing sibling session (multi-tab race loser)",
				slog.String("userID", string(rt.UserID)),
				slog.String("tokenID", rt.ID))
			return rt, true, nil
		}
		slog.Info("refresh rejected: token rotated outside the grace window (replay or long-stale client)",
			slog.String("userID", string(rt.UserID)),
			slog.String("tokenID", rt.ID))
		return nil, false, ErrInvalidCredentials
	}

	return rt, false, nil
}

// markRefreshTokenUsed consumes the (already validated) refresh token. Split
// from validateRefreshToken so a rejected parental PIN gate does NOT burn the
// token: the client must be able to fetch a PIN proof and retry with the SAME
// refresh token. Burning it would log the user out - and the retry would look
// like a replay, purging every session of that user.
func (s *CommandService) markRefreshTokenUsed(ctx context.Context, rt *user.RefreshToken) error {
	claimed, markErr := s.refreshTkns.MarkUsedIfNotUsed(ctx, rt.ID)
	if markErr != nil {
		return fmt.Errorf("CommandService.RefreshToken: %w", markErr)
	}
	if !claimed {
		// Two devices (e.g. web + mobile) both fired /auth/refresh with the
		// same not-yet-rotated token; one of them won MarkUsedIfNotUsed and
		// the other landed here. The loser is NOT a replayed attacker - it
		// is one of the user's own clients that simply lost the optimistic
		// race. Reject it, but DO NOT purge the user's other sessions; the
		// in-flight refresh on the other client (or the next AppState resume
		// retry) will use the new token and succeed.
		slog.Info("refresh rejected: concurrent claim race",
			slog.String("userID", string(rt.UserID)),
			slog.String("tokenID", rt.ID))
		return ErrInvalidCredentials
	}
	return nil
}

// collectPermissions flattens all role permissions for the user. Errors
// are logged and swallowed - the caller gets an access token with an
// empty perm set rather than an outright 500.
func (s *CommandService) collectPermissions(ctx context.Context, userID shared.UserID) []string {
	roles, err := s.userRoles.GetRolesByUserID(ctx, userID)
	if err != nil {
		slog.Warn("get roles by user failed", slog.String("userID", string(userID)), slog.Any("err", err))
		return nil
	}
	var perms []string
	for _, r := range roles {
		perms = append(perms, r.Permissions...)
	}
	return perms
}

// findProfileByID returns the profile with the given ID, or nil.
func findProfileByID(profs []*user.Profile, id string) *user.Profile {
	for _, p := range profs {
		if string(p.ID) == id {
			return p
		}
	}
	return nil
}

// resolveActiveProfileID picks the profile ID to embed in the access token.
// Returns ok=false only when the caller asked for a specific profile that
// does not belong to them (authorization failure). When no profile matches
// but the caller didn't request one, falls back to the default profile then
// the first profile; ok remains true in that case.
func resolveActiveProfileID(profs []*user.Profile, requestedProfileID string) (string, bool) {
	if requestedProfileID != "" {
		for _, p := range profs {
			if string(p.ID) == requestedProfileID {
				return requestedProfileID, true
			}
		}
		return "", false
	}
	for _, p := range profs {
		if p.IsDefault {
			return string(p.ID), true
		}
	}
	if len(profs) > 0 {
		return string(profs[0].ID), true
	}
	return "", true
}

func (s *CommandService) lookupProfileLocale(ctx context.Context, profileID string) string {
	if profileID == "" {
		return ""
	}
	prof, err := s.profiles.GetByID(ctx, shared.ProfileID(profileID))
	if err != nil || prof == nil {
		return ""
	}
	return prof.Locale
}

func (s *CommandService) Logout(ctx context.Context, userID string) error {
	ctx, span := tracing.StartSpan(ctx, "app.user.Logout")
	defer span.End()
	if err := s.refreshTkns.DeleteByUserID(ctx, shared.UserID(userID)); err != nil {
		return fmt.Errorf("CommandService.Logout: %w", err)
	}
	return nil
}

const maxRefreshTokenRetries = 3

func (s *CommandService) generateAndStoreRefreshToken(ctx context.Context, userID, profileID string, ttlOverride ...time.Duration) (string, error) {
	ttl := s.refreshTokenTTL
	if len(ttlOverride) > 0 && ttlOverride[0] > 0 {
		ttl = ttlOverride[0]
	}
	var boundProfile *shared.ProfileID
	if profileID != "" {
		pid := shared.ProfileID(profileID)
		boundProfile = &pid
	}
	for range maxRefreshTokenRetries {
		token, err := s.tokens.GenerateRefreshToken()
		if err != nil {
			return "", fmt.Errorf("generate refresh token: %w", err)
		}

		h := sha256.Sum256([]byte(token))
		rt := &user.RefreshToken{
			ID:        id.New("rt-"),
			UserID:    shared.UserID(userID),
			TokenHash: hex.EncodeToString(h[:]),
			ProfileID: boundProfile,
			ExpiresAt: time.Now().Add(ttl),
			CreatedAt: time.Now(),
		}
		err = s.refreshTkns.Create(ctx, rt)
		if err == nil {
			return token, nil
		}
		if !errors.Is(err, shared.ErrConflict) {
			return "", fmt.Errorf("store refresh token: %w", err)
		}
	}
	return "", errors.New("store refresh token: duplicate hash after retries")
}

func (s *CommandService) StartCleanup(ctx context.Context) {
	s.StartCleanupWithInterval(ctx, defaultCleanupInterval)
}

func (s *CommandService) StartCleanupWithInterval(ctx context.Context, interval time.Duration) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic in user cleanup", slog.Any("recover", r))
			}
		}()
		s.runCleanup(ctx)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runCleanup(ctx)
			}
		}
	}()
}

func (s *CommandService) runCleanup(ctx context.Context) {
	if err := s.refreshTkns.DeleteExpired(ctx); err != nil {
		slog.Warn("refresh token cleanup", slog.Any("err", err))
	}
	if err := s.quickConnect.DeleteExpired(ctx); err != nil {
		slog.Warn("quick connect cleanup", slog.Any("err", err))
	}
	if s.resetTokens != nil {
		if err := s.resetTokens.DeleteExpired(ctx); err != nil {
			slog.Warn("password reset token cleanup", slog.Any("err", err))
		}
	}
}

func (s *CommandService) IsPasswordRecoveryEnabled(ctx context.Context) bool {
	if !s.smtpConfigured || s.serverCfg == nil {
		return false
	}
	entry, err := s.serverCfg.Get(ctx, "auth.password_recovery_enabled")
	if err != nil {
		return false
	}
	return entry.Value == "true"
}

const passwordResetTokenTTL = 30 * time.Minute

func (s *CommandService) RequestPasswordReset(ctx context.Context, email string) error {
	ctx, span := tracing.StartSpan(ctx, "app.user.RequestPasswordReset")
	defer span.End()
	if !s.IsPasswordRecoveryEnabled(ctx) {
		return ErrPasswordRecoveryDisabled
	}
	if s.resetTokens == nil {
		return ErrPasswordRecoveryDisabled
	}

	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		return nil // no leak
	}
	if !u.Enabled {
		return nil
	}

	if err := s.resetTokens.DeleteByUserID(ctx, u.ID); err != nil {
		slog.Warn("cleanup old reset tokens", slog.String("userID", string(u.ID)), slog.Any("err", err))
	}

	rawToken := make([]byte, resetTokenBytes)
	if _, err := rand.Read(rawToken); err != nil {
		return fmt.Errorf("generate reset token: %w", err)
	}
	rawHex := hex.EncodeToString(rawToken)
	h := sha256.Sum256([]byte(rawHex))

	t := &user.PasswordResetToken{
		ID:        id.New("prt-"),
		UserID:    u.ID,
		TokenHash: hex.EncodeToString(h[:]),
		ExpiresAt: time.Now().Add(passwordResetTokenTTL),
		CreatedAt: time.Now(),
	}
	if err := s.resetTokens.Create(ctx, t); err != nil {
		return fmt.Errorf("store reset token: %w", err)
	}

	evt := user.PasswordResetRequested{
		UserID:     u.ID,
		Email:      u.Email,
		ResetToken: rawHex,
		ExpiresAt:  t.ExpiresAt,
		OccurredAt: time.Now(),
	}
	if err := s.events.Publish(ctx, evt); err != nil {
		slog.Warn("publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *CommandService) ResetPassword(ctx context.Context, rawToken, newPassword string) error {
	ctx, span := tracing.StartSpan(ctx, "app.user.ResetPassword")
	defer span.End()
	if s.resetTokens == nil {
		return ErrResetTokenInvalid
	}

	h := sha256.Sum256([]byte(rawToken))
	hash := hex.EncodeToString(h[:])

	t, err := s.resetTokens.GetByTokenHash(ctx, hash)
	if err != nil {
		return ErrResetTokenInvalid
	}
	if !t.IsValid() {
		return ErrResetTokenInvalid
	}

	if len(newPassword) < minPasswordLength {
		return ErrPasswordTooShort
	}

	u, err := s.users.GetByID(ctx, shared.UserID(t.UserID))
	if err != nil {
		return ErrResetTokenInvalid
	}

	newHash, err := s.hasher.Hash(newPassword)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	u.PasswordHash = newHash
	u.UpdatedAt = time.Now()

	// Atomic: password update, token mark-used and refresh-token eviction must
	// commit together. A MarkUsed failure would otherwise leave the token
	// reusable (TOCTOU); a non-atomic refresh-token delete would leave a
	// surviving refresh token able to mint fresh access tokens (the epoch bump
	// below only invalidates outstanding access tokens, not refresh tokens),
	// silently bypassing the reset on a DB hiccup.
	apply := func(ctx context.Context) error {
		if err := s.users.Update(ctx, u); err != nil {
			return fmt.Errorf("update user: %w", err)
		}
		if err := s.resetTokens.MarkUsed(ctx, t.ID); err != nil {
			return fmt.Errorf("mark reset token used: %w", err)
		}
		if err := s.refreshTkns.DeleteByUserID(ctx, u.ID); err != nil {
			return fmt.Errorf("delete refresh tokens: %w", err)
		}
		return nil
	}
	var txErr error
	if s.uow != nil {
		txErr = s.uow.Do(ctx, apply)
	} else {
		txErr = apply(ctx)
	}
	if txErr != nil {
		return fmt.Errorf("CommandService.ResetPassword: %w", txErr)
	}
	if s.tokenRevoker != nil {
		if err := s.tokenRevoker.Revoke(ctx, string(u.ID)); err != nil {
			slog.Warn("revoke access tokens after password reset", slog.Any("err", err))
		}
	}

	evt := user.PasswordChanged{UserID: u.ID, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evt); err != nil {
		slog.Warn("publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
	}
	return nil
}

// loginAttemptLimiter tracks failed login attempts per username.
type loginAttemptLimiter struct {
	mu       sync.Mutex
	attempts map[string]*attemptEntry
	maxFails int
	maxSize  int
	window   time.Duration
}

type attemptEntry struct {
	count    int
	lockedAt time.Time
	lastSeen time.Time
}

func newLoginAttemptLimiter(maxFails int, window time.Duration) *loginAttemptLimiter {
	return &loginAttemptLimiter{
		attempts: make(map[string]*attemptEntry),
		maxFails: maxFails,
		maxSize:  loginLimiterMaxEntries,
		window:   window,
	}
}

// evictLocked removes expired entries, then oldest if still over cap. Caller holds mu.
func (l *loginAttemptLimiter) evictLocked(now time.Time) {
	// pass 1: remove expired
	for k, e := range l.attempts {
		if !e.lockedAt.IsZero() && now.Sub(e.lockedAt) > l.window {
			delete(l.attempts, k)
		} else if e.lockedAt.IsZero() && now.Sub(e.lastSeen) > l.window {
			delete(l.attempts, k)
		}
	}
	if len(l.attempts) < l.maxSize {
		return
	}
	// pass 2: evict oldest entries until under cap
	for len(l.attempts) >= l.maxSize {
		oldestKey := ""
		var oldestTime time.Time
		for k, e := range l.attempts {
			if oldestKey == "" || e.lastSeen.Before(oldestTime) {
				oldestKey = k
				oldestTime = e.lastSeen
			}
		}
		if oldestKey == "" {
			break
		}
		delete(l.attempts, oldestKey)
	}
}

func (l *loginAttemptLimiter) isLocked(username string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.attempts[username]
	if !ok {
		return false
	}
	if e.lockedAt.IsZero() {
		return false
	}
	if time.Since(e.lockedAt) < l.window {
		return true
	}
	delete(l.attempts, username)
	return false
}

func (l *loginAttemptLimiter) reset(username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, username)
}

// IsLocked implements user.LoginAttemptRepository.
func (l *loginAttemptLimiter) IsLocked(_ context.Context, username string) (bool, error) {
	return l.isLocked(username), nil
}

// Record implements user.LoginAttemptRepository.
// maxFails and window override the limiter's own values when provided.
func (l *loginAttemptLimiter) Record(_ context.Context, username string, maxFails int, window time.Duration) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()

	if len(l.attempts) >= l.maxSize {
		l.evictLocked(now)
	}

	e, ok := l.attempts[username]
	if !ok {
		e = &attemptEntry{}
		l.attempts[username] = e
	} else if now.Sub(e.lastSeen) > window {
		e = &attemptEntry{}
		l.attempts[username] = e
	}
	e.count++
	e.lastSeen = now
	if e.count >= maxFails {
		e.lockedAt = now
	}
	return nil
}

// Reset implements user.LoginAttemptRepository.
func (l *loginAttemptLimiter) Reset(_ context.Context, username string) error {
	l.reset(username)
	return nil
}

var _ user.LoginAttemptRepository = (*loginAttemptLimiter)(nil)

// PurgeStale drops settled entries. The in-memory limiter already evicts on
// every write (expired first, then oldest over the cap), so this is a no-op
// kept for interface parity with the Postgres repository - which has no such
// self-eviction and needs the sweep.
func (l *loginAttemptLimiter) PurgeStale(_ context.Context, _ time.Duration) (int64, error) {
	return 0, nil
}

// loginAttemptStaleAfter is how long after its last write a settled row is
// considered dead. Comfortably past any rate-limit window (minutes), so a row
// is only ever dropped once it can no longer influence a decision.
const loginAttemptStaleAfter = 24 * time.Hour

// StartLoginAttemptCleanup sweeps settled login-attempt rows hourly until ctx
// is done.
//
// Hourly rather than daily because this table takes UNAUTHENTICATED writes: the
// unknown-user path records an attempt on purpose, so that a missing account
// costs the same as a wrong password and cannot be found by timing. The price
// is one row per distinct username an attacker tries, and a dictionary run is
// exactly the traffic this table defends against - so the cleanup has to keep
// pace with an attack, not with a maintenance window.
func (s *CommandService) StartLoginAttemptCleanup(ctx context.Context) {
	if s.loginAttempts == nil {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("login-attempt cleanup: panic recovered", slog.Any("panic", r))
			}
		}()
		sweep := func() {
			n, err := s.loginAttempts.PurgeStale(ctx, loginAttemptStaleAfter)
			if err != nil {
				slog.Warn("login-attempt cleanup failed", slog.Any("err", err))
				return
			}
			if n > 0 {
				slog.Info("login-attempt cleanup", slog.Int64("removed", n))
			}
		}
		sweep()
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sweep()
			}
		}
	}()
}
