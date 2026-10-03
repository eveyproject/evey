package user

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"slices"
	"strings"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/id"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// auth.go holds the authentication surface: password login, admin
// impersonation, refresh-token rotation (with the multi-tab grace and the
// parental PIN gate), permission collection and the refresh-token store
// helpers.

// safeTokenPrefix returns the first 8 chars of a token (safe for logs).
func safeTokenPrefix(token string) string {
	if len(token) < 8 {
		return "<short>"
	}
	return token[:8] + "…"
}

// dummyLoginHash is a valid cost-12 bcrypt hash of a throwaway string. It is
// verified against the submitted password when the username does NOT exist, so
// a failed login costs the same ~100ms whether or not the account is real -
// closing the timing oracle that would otherwise enumerate usernames. It is
// never a usable credential (no account stores it).
const dummyLoginHash = "$2a$12$CCENmDmu1fye2Bn2DUS7G.4pIkth0OyzzFCkIQvblCV9aye/etkwO"

// loginAttemptKey is the lockout counter's key for a username: the lookup is
// case-insensitive (lower(username), migration 000169), so the counter must
// be too, or "Alice" would get five fresh tries after "alice" was locked.
func loginAttemptKey(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

type loginClientIPKey struct{}

// WithLoginClientIP carries the resolved client IP to Login, where it scopes
// the hard lockout. Set by the gRPC client-info interceptor.
func WithLoginClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, loginClientIPKey{}, ip)
}

// LoginClientIP returns what WithLoginClientIP stored ("" when nothing).
func LoginClientIP(ctx context.Context) string {
	ip, _ := ctx.Value(loginClientIPKey{}).(string)
	return ip
}

// loginAttemptKeys returns the (IP, user) key the hard lock counts on and the
// user-only key of the higher distributed-guessing cap. A missing or unparsable
// IP falls into one shared "unknown" bucket, never a fresh one.
func loginAttemptKeys(ctx context.Context, username string) (pairKey, userKey string) {
	ip := LoginClientIP(ctx)
	if parsed := net.ParseIP(strings.TrimSpace(ip)); parsed != nil {
		ip = parsed.String()
	} else {
		ip = "unknown"
	}
	userKey = "user:" + loginAttemptKey(username)
	return "ip:" + ip + "|" + userKey, userKey
}

// publishLoginFailed announces a refused login (webhook user.login.failed).
func (s *CommandService) publishLoginFailed(ctx context.Context, username, reason string) {
	evt := user.LoginFailed{Username: username, Reason: reason, AuthMethod: "local", OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evt); err != nil {
		slog.WarnContext(ctx, "publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
	}
}

func (s *CommandService) loginLockExempt(username string) bool {
	return s.lockExempt != "" && strings.EqualFold(strings.TrimSpace(username), s.lockExempt)
}

func (s *CommandService) loginLocked(ctx context.Context, username string, keys ...string) bool {
	if s.loginLockExempt(username) {
		return false
	}
	for _, k := range keys {
		locked, err := s.loginAttempts.IsLocked(ctx, k)
		if err != nil {
			slog.WarnContext(ctx, "login attempt IsLocked", slog.String("username", username), slog.Any("err", err))
		}
		if locked {
			return true
		}
	}
	return false
}

func (s *CommandService) recordLoginFailure(ctx context.Context, username, pairKey, userKey string) {
	if s.loginLockExempt(username) {
		return
	}
	if err := s.loginAttempts.Record(ctx, pairKey, s.loginMaxFails, s.loginWindow); err != nil {
		slog.WarnContext(ctx, "login attempt Record", slog.String("username", username), slog.Any("err", err))
	}
	if err := s.loginAttempts.Record(ctx, userKey, s.loginUserMaxFails, s.loginWindow); err != nil {
		slog.WarnContext(ctx, "login attempt Record", slog.String("username", username), slog.Any("err", err))
	}
}

func (s *CommandService) Login(ctx context.Context, username, password string, ttlOverride ...time.Duration) (accessToken, refreshToken string, err error) {
	ctx, span := tracing.StartSpan(ctx, "app.user.Login")
	defer span.End()
	username = strings.TrimSpace(username)
	password = normalizePassword(password)
	// Hard lock per (client IP, user): a stranger's failures must not lock the
	// owner out. The user-wide cap, far higher, still stops distributed guessing.
	pairKey, userKey := loginAttemptKeys(ctx, username)
	if s.loginLocked(ctx, username, pairKey, userKey) {
		s.publishLoginFailed(ctx, username, "locked")
		return "", "", ErrAccountLocked
	}

	u, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		// Constant-time defense against username enumeration: a missing user must
		// cost the same as a wrong password. Without this, the bcrypt verify below
		// runs ONLY for existing users, so the ~100ms (cost-12) delta leaks which
		// usernames exist. Burn an equivalent bcrypt verify against a fixed hash.
		_, _ = s.hasher.Verify(password, dummyLoginHash) //nolint:errcheck // constant-time burn; result intentionally discarded
		s.recordLoginFailure(ctx, username, pairKey, userKey)
		// Per-attempt log (username only, never the password) so a
		// credential-stuffing run is searchable, not just inferable from the
		// rate-limit store's aggregate.
		slog.WarnContext(ctx, "login failed", slog.String("username", username), slog.String("reason", "unknown_user"))
		s.publishLoginFailed(ctx, username, "unknown_user")
		return "", "", ErrInvalidCredentials
	}
	// Verify BEFORE the CanLogin gate so a disabled account costs the same bcrypt
	// as an enabled account with a wrong password. Returning ErrUserDisabled
	// without running the hash lets timing separate "exists & disabled" from the
	// other states (the error is flattened to a generic message above the HTTP
	// boundary, so timing is the only remaining oracle).
	ok, err := s.hasher.Verify(password, u.PasswordHash)
	if !u.CanLogin() {
		s.publishLoginFailed(ctx, username, "disabled")
		return "", "", ErrUserDisabled
	}
	if err != nil || !ok {
		s.recordLoginFailure(ctx, username, pairKey, userKey)
		slog.WarnContext(ctx, "login failed", slog.String("username", username), slog.String("reason", "bad_password"))
		s.publishLoginFailed(ctx, username, "bad_password")
		return "", "", ErrInvalidCredentials
	}
	// Only this address's counter: the user-wide one decays with its window, so a
	// success does not hand a distributed run a fresh budget.
	if resetErr := s.loginAttempts.Reset(ctx, pairKey); resetErr != nil {
		slog.WarnContext(ctx, "login attempt Reset", slog.String("username", username), slog.Any("err", resetErr))
	}

	perms := s.collectPermissions(ctx, u.ID)

	var defaultProfileID string
	// Fail CLOSED on a lookup error. An empty ProfileID is read as "no parental
	// restriction" by every gate in the server, so continuing past this fault
	// would issue an unrestricted token. Same reasoning as RefreshToken.
	profs, pErr := s.profiles.GetByUserID(ctx, u.ID)
	if pErr != nil {
		slog.WarnContext(ctx, "profile lookup failed, refusing rather than issuing a profile-less token",
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

	refreshToken, _, err = s.generateAndStoreRefreshToken(ctx, string(u.ID), defaultProfileID, ttlOverride...)
	if err != nil {
		return "", "", fmt.Errorf("CommandService.Login: %w", err)
	}

	evtLogin := user.LoginSucceeded{UserID: u.ID, AuthMethod: "local", OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtLogin); err != nil {
		slog.WarnContext(ctx, "publish event", slog.String("event", evtLogin.EventName()), slog.Any("err", err))
	}
	if err := s.users.TouchLastLogin(ctx, u.ID, evtLogin.OccurredAt); err != nil {
		slog.WarnContext(ctx, "touch last_login", slog.String("user_id", string(u.ID)), slog.Any("err", err))
	}
	s.touchProfileActive(ctx, defaultProfileID, evtLogin.OccurredAt)
	return accessToken, refreshToken, nil
}

// touchProfileActive records that a token was just minted for profileID, which
// is what the admin panel shows as the profile's last activity. Best-effort:
// worth a log line, never a failed login or refresh. Impersonate deliberately
// does not call it - an admin looking around is not the profile being used.
func (s *CommandService) touchProfileActive(ctx context.Context, profileID string, at time.Time) {
	if profileID == "" {
		return
	}
	if err := s.profiles.TouchLastActive(ctx, shared.ProfileID(profileID), at); err != nil {
		slog.WarnContext(ctx, "touch profile last_active", slog.String("profile_id", profileID), slog.Any("err", err))
	}
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
		slog.WarnContext(ctx, "profile lookup failed, refusing rather than issuing a profile-less token",
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

	refreshToken, _, err = s.generateAndStoreRefreshToken(ctx, string(u.ID), defaultProfileID, ttlOverride...)
	if err != nil {
		return "", "", fmt.Errorf("CommandService.GenerateTokensForUser: %w", err)
	}
	s.touchProfileActive(ctx, defaultProfileID, time.Now())
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
		slog.WarnContext(ctx, "refresh: profile lookup failed, refusing rather than dropping the profile binding",
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

		// Stream/cast tokens minted under the OLD profile outlive the switch
		// (their claims are frozen at mint time). Only a RESTRICTED target can
		// be bypassed by them, so only that switch purges them: an adult picking
		// their own profile after a login must not cut the cast running on the TV.
		if s.tokenRevoker != nil && target.IsParentalRestricted() {
			if revokeErr := s.tokenRevoker.Revoke(ctx, string(u.ID)); revokeErr != nil {
				slog.WarnContext(ctx, "revoke stream tokens on profile switch", slog.Any("err", revokeErr))
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
		var newID string
		newRefresh, newID, genErr = s.generateAndStoreRefreshToken(ctx, string(u.ID), activeProfileID, ttlOverride...)
		if genErr != nil {
			return fmt.Errorf("CommandService.RefreshToken: %w", genErr)
		}
		// Leave the breadcrumb: which row this one was rotated into. It is what
		// lets a client whose answer was lost come back and be believed (see
		// validateRefreshToken). Best-effort - failing the whole refresh over a
		// bookkeeping write would log the viewer out for certain, to protect
		// them from a case where they only MIGHT be.
		if !graced {
			if setErr := s.refreshTkns.SetSuccessor(ctx, rt.ID, newID); setErr != nil {
				slog.WarnContext(ctx, "refresh: recording the rotation successor failed",
					slog.String("tokenID", rt.ID), slog.Any("err", setErr))
			}
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

	// Every refresh, silent or switch: a silent one means the app is open
	// under this profile right now, which is exactly what "last active" means.
	s.touchProfileActive(ctx, activeProfileID, time.Now())
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
		slog.InfoContext(ctx, "refresh rejected: token not found",
			slog.String("tokenPrefix", safeTokenPrefix(token)),
			slog.String("token_info", s.tokens.DescribeRefreshToken(token)))
		return nil, false, ErrInvalidCredentials
	}

	if rt.IsExpired() {
		slog.InfoContext(ctx, "refresh rejected: token expired",
			slog.String("userID", string(rt.UserID)),
			slog.String("tokenID", rt.ID),
			slog.Time("expiredAt", rt.ExpiresAt))
		if delErr := s.refreshTkns.DeleteByID(ctx, rt.ID); delErr != nil {
			slog.WarnContext(ctx, "delete refresh token failed", slog.String("tokenID", rt.ID), slog.Any("err", delErr))
		}
		return nil, false, ErrInvalidCredentials
	}

	if rt.Used {
		// nil UsedAt (legacy row claimed before the column existed) fails
		// closed: no timestamp, no grace.
		if rt.UsedAt != nil && time.Since(*rt.UsedAt) <= refreshRotationGrace {
			slog.InfoContext(ctx, "refresh: rotated token within grace, issuing sibling session (multi-tab race loser)",
				slog.String("userID", string(rt.UserID)),
				slog.String("tokenID", rt.ID))
			return rt, true, nil
		}
		// Past the grace, one case is still not a replay: the rotation answer
		// never reached the client. The app was killed between our commit and
		// its own write (an install, a crash at boot - a television is killed
		// far more often than a browser), so it still holds this token and the
		// successor we issued has never been used by anyone. Measured on the
		// living-room box 2026-09-19: that client is logged out for good, with
		// no keyboard to answer the login screen it lands on.
		//
		// A thief replaying a consumed token has no such alibi: the legitimate
		// client did receive the successor and did use it.
		if s.rotationWasInterrupted(ctx, rt) {
			slog.InfoContext(ctx, "refresh: rotation answer was lost, reissuing (successor never used)",
				slog.String("userID", string(rt.UserID)),
				slog.String("tokenID", rt.ID))
			return rt, true, nil
		}
		slog.InfoContext(ctx, "refresh rejected: token rotated outside the grace window (replay or long-stale client)",
			slog.String("userID", string(rt.UserID)),
			slog.String("tokenID", rt.ID))
		return nil, false, ErrInvalidCredentials
	}

	return rt, false, nil
}

// rotationWasInterrupted reports whether the successor issued for this
// consumed token was never used - the signature of an answer lost in flight
// rather than of a replay. The orphan successor is burned on the way out: the
// caller issues a sibling session instead, and whoever might have intercepted
// that lost token on the wire cannot spend it afterwards.
func (s *CommandService) rotationWasInterrupted(ctx context.Context, rt *user.RefreshToken) bool {
	if rt.SuccessorID == nil {
		return false
	}
	successor, err := s.refreshTkns.GetByID(ctx, *rt.SuccessorID)
	if err != nil || successor == nil {
		// Swept, or never stored: nothing proves anything, stay closed.
		return false
	}
	if successor.Used || successor.IsExpired() {
		return false
	}
	if delErr := s.refreshTkns.DeleteByID(ctx, successor.ID); delErr != nil {
		// Could not neutralise the orphan - refuse rather than leave two live
		// tokens for one session.
		slog.WarnContext(ctx, "refresh: could not burn the orphaned successor, refusing",
			slog.String("tokenID", rt.ID), slog.String("successorID", successor.ID), slog.Any("err", delErr))
		return false
	}
	return true
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
		slog.InfoContext(ctx, "refresh rejected: concurrent claim race",
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
		slog.WarnContext(ctx, "get roles by user failed", slog.String("userID", string(userID)), slog.Any("err", err))
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

// Logout ends ONE device's session when it presents its refresh token: that
// row alone is deleted and the other devices keep playing.
//
// A logout that names NO session ends none. It used to fall back to an
// account-wide revoke, which signed the television and the browser out
// because a phone had been signed out - and a client reaches that fallback
// without meaning to, whenever its stored token cannot be read. The browser
// never does (the HTTP layer injects its cookie into the body), so the
// fallback only ever punished the honest case. A device that wants its own
// session ended presents the token that names it; anything else is cleared
// locally and the row expires on its own.
func (s *CommandService) Logout(ctx context.Context, userID, refreshToken string) error {
	ctx, span := tracing.StartSpan(ctx, "app.user.Logout")
	defer span.End()
	if refreshToken == "" {
		slog.InfoContext(ctx, "logout without a refresh token: no session named, none ended",
			slog.String("userID", userID))
		return nil
	}
	h := sha256.Sum256([]byte(refreshToken))
	rt, err := s.refreshTkns.GetByTokenHash(ctx, hex.EncodeToString(h[:]))
	if err != nil || string(rt.UserID) != userID {
		// A token this account does not own, or one already gone: nothing to
		// end, and certainly not everything else.
		slog.InfoContext(ctx, "logout: the presented token names no session of this account",
			slog.String("userID", userID))
		return nil
	}
	if delErr := s.refreshTkns.DeleteByID(ctx, rt.ID); delErr != nil {
		return fmt.Errorf("CommandService.Logout: %w", delErr)
	}
	// Deliberately NO account-wide revoke here. The only revoker available
	// (compositeTokenRevoker) works per USER: it bumps the JWT epoch and cuts
	// every stream token of the account, which would stop the film playing on
	// the television because a phone was signed out. The signed-out device's
	// own access token dies with its clock (minutes) and its stream tokens
	// with theirs; ending the account's other sessions is what DisableUser and
	// ChangePassword are for.
	return nil
}

const maxRefreshTokenRetries = 3

func (s *CommandService) generateAndStoreRefreshToken(ctx context.Context, userID, profileID string, ttlOverride ...time.Duration) (string, string, error) {
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
			return "", "", fmt.Errorf("generate refresh token: %w", err)
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
			return token, rt.ID, nil
		}
		if !errors.Is(err, shared.ErrConflict) {
			return "", "", fmt.Errorf("store refresh token: %w", err)
		}
	}
	return "", "", errors.New("store refresh token: duplicate hash after retries")
}

// normalizePassword is applied wherever a password is hashed AND at login:
// the two sides must agree, or a password typed with a stray space (phone
// keyboards) hashes one way and verifies another.
func normalizePassword(p string) string {
	return strings.TrimSpace(p)
}
