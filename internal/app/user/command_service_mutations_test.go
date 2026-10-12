package user

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/shared/fakes"
	"github.com/eveyproject/evey/internal/domain/user"
	ufakes "github.com/eveyproject/evey/internal/domain/user/fakes"
)

// captureSlogBuffer redirects slog output to a buffer; returns buffer + restore fn.
func captureSlogBuffer(t *testing.T) (*bytes.Buffer, func()) {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	return &buf, func() { slog.SetDefault(prev) }
}

// --- wrapper fakes for targeted error injection ---

type rtRepoDeleteByIDFail struct {
	*ufakes.FakeRefreshTokenRepository
}

func (r *rtRepoDeleteByIDFail) DeleteByID(_ context.Context, _ string) error {
	return errors.New("delete by id boom")
}

type rtRepoMarkUsedFail struct {
	*ufakes.FakeRefreshTokenRepository
}

func (r *rtRepoMarkUsedFail) MarkUsedIfNotUsed(_ context.Context, _ string) (bool, error) {
	return false, errors.New("mark used boom")
}

type rtRepoDeleteByUserIDFail struct {
	*ufakes.FakeRefreshTokenRepository
}

func (r *rtRepoDeleteByUserIDFail) DeleteByUserID(_ context.Context, _ shared.UserID) error {
	return errors.New("delete by user boom")
}

type resetRepoDeleteExpiredFail struct {
	*ufakes.FakePasswordResetTokenRepository
}

func (r *resetRepoDeleteExpiredFail) DeleteExpired(_ context.Context) error {
	return errors.New("reset cleanup boom")
}

type qcRepoDeleteExpiredFail struct {
	*ufakes.FakeQuickConnectRepository
}

func (r *qcRepoDeleteExpiredFail) DeleteExpired(_ context.Context) error {
	return errors.New("qc cleanup boom")
}

// --- Setup: ID prefix mutations (lines 116, 134, 148) ---

func TestSetup_AssertsIDPrefixes(t *testing.T) {
	d := newAuthTestDeps()
	u, err := d.svc.Setup(context.Background(), "admin", "longpassword", "a@a.com")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(u.ID), "usr-") {
		t.Fatalf("expected user ID prefix 'usr-', got %q", u.ID)
	}
	profs, _ := d.profiles.GetByUserID(context.Background(), u.ID)
	if len(profs) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profs))
	}
	if !strings.HasPrefix(string(profs[0].ID), "prf-") {
		t.Fatalf("expected profile ID prefix 'prf-', got %q", profs[0].ID)
	}
	if profs[0].Name != u.Username {
		t.Fatalf("profile name should equal username, got %q", profs[0].Name)
	}
	if !profs[0].IsDefault {
		t.Fatal("profile should be default")
	}
}

// --- Login/RefreshToken/GenerateTokensForUser: assert locale + perms propagated ---
// (kills mutations on lines 180, 201, 202, 241, 262, 263, 330, 352, 353)

func TestLogin_ClaimsIncludeLocaleAndMultiRolePerms(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", true, true)
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Main", IsDefault: true, Locale: "fr"})
	_ = d.roles.Create(ctx, &user.Role{ID: "r1", Name: "a", Permissions: []string{"read"}})
	_ = d.roles.Create(ctx, &user.Role{ID: "r2", Name: "b", Permissions: []string{"write", "admin"}})
	_ = d.userRoles.AssignRole(ctx, "u1", "r1")
	_ = d.userRoles.AssignRole(ctx, "u1", "r2")

	access, _, err := d.svc.Login(ctx, "alice", "pw")
	if err != nil {
		t.Fatal(err)
	}
	claims, _ := d.tokens.ValidateAccessToken(access)
	if claims.Locale != "fr" {
		t.Fatalf("expected locale fr, got %q", claims.Locale)
	}
	if claims.Username != "alice" {
		t.Fatalf("expected username alice, got %q", claims.Username)
	}
	if !claims.IsAdmin {
		t.Fatal("expected IsAdmin true")
	}
	if len(claims.Permissions) != 3 {
		t.Fatalf("expected 3 perms from 2 roles, got %v", claims.Permissions)
	}
}

func TestGenerateTokensForUser_ClaimsIncludeLocale(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "M", IsDefault: true, Locale: "es"})

	access, _, err := d.svc.GenerateTokensForUser(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	claims, _ := d.tokens.ValidateAccessToken(access)
	if claims.Locale != "es" {
		t.Fatalf("expected locale es, got %q", claims.Locale)
	}
	if claims.UserID != "u1" || claims.Username != "alice" {
		t.Fatalf("bad claims: %+v", claims)
	}
}

func TestRefreshToken_ClaimsIncludeLocaleAndPerms(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", true, true)
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "M", IsDefault: true, Locale: "zh"})
	_ = d.roles.Create(ctx, &user.Role{ID: "r1", Name: "a", Permissions: []string{"p1", "p2"}})
	_ = d.userRoles.AssignRole(ctx, "u1", "r1")

	raw := "refresh-locale-tok"
	h := sha256.Sum256([]byte(raw))
	_ = d.refreshTkns.Create(ctx, &user.RefreshToken{
		ID: "rt1", UserID: "u1", TokenHash: hex.EncodeToString(h[:]),
		ExpiresAt: time.Now().Add(time.Hour),
	})

	access, _, err := d.svc.RefreshToken(ctx, raw, "", "")
	if err != nil {
		t.Fatal(err)
	}
	claims, _ := d.tokens.ValidateAccessToken(access)
	if claims.Locale != "zh" {
		t.Fatalf("expected locale zh, got %q", claims.Locale)
	}
	if len(claims.Permissions) != 2 {
		t.Fatalf("expected 2 perms, got %v", claims.Permissions)
	}
	if !claims.IsAdmin {
		t.Fatal("expected IsAdmin")
	}
}

// --- RefreshToken: IsExpired path logs DeleteByID error (line 297-301) ---

func TestRefreshToken_ExpiredAndDeleteByIDFailsLogsWarning(t *testing.T) {
	buf, restore := captureSlogBuffer(t)
	defer restore()

	users := ufakes.NewFakeUserRepository()
	profiles := ufakes.NewFakeProfileRepository()
	inner := ufakes.NewFakeRefreshTokenRepository()
	rt := &rtRepoDeleteByIDFail{FakeRefreshTokenRepository: inner}
	roles := ufakes.NewFakeRoleRepository()
	userRoles := ufakes.NewFakeUserRoleRepository(roles)
	hasher := ufakes.NewFakePasswordHasher()
	tokens := ufakes.NewFakeTokenGenerator()
	events := fakes.NewFakeEventBus()
	svc := NewCommandService(users, profiles, rt, roles, userRoles, hasher, tokens, events,
		time.Hour, ufakes.NewFakeQuickConnectRepository())

	ctx := context.Background()
	raw := "expired-delete-fail"
	h := sha256.Sum256([]byte(raw))
	_ = inner.Create(ctx, &user.RefreshToken{
		ID: "rt1", UserID: "u1", TokenHash: hex.EncodeToString(h[:]),
		ExpiresAt: time.Now().Add(-time.Hour),
	})

	_, _, err := svc.RefreshToken(ctx, raw, "", "")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
	if !strings.Contains(buf.String(), "delete refresh token failed") {
		t.Fatalf("expected warn log, got %q", buf.String())
	}
}

// --- RefreshToken: MarkUsedIfNotUsed error returns wrapped error (line 305-308) ---

func TestRefreshToken_MarkUsedError(t *testing.T) {
	users := ufakes.NewFakeUserRepository()
	profiles := ufakes.NewFakeProfileRepository()
	inner := ufakes.NewFakeRefreshTokenRepository()
	rt := &rtRepoMarkUsedFail{FakeRefreshTokenRepository: inner}
	roles := ufakes.NewFakeRoleRepository()
	userRoles := ufakes.NewFakeUserRoleRepository(roles)
	hasher := ufakes.NewFakePasswordHasher()
	tokens := ufakes.NewFakeTokenGenerator()
	events := fakes.NewFakeEventBus()
	svc := NewCommandService(users, profiles, rt, roles, userRoles, hasher, tokens, events,
		time.Hour, ufakes.NewFakeQuickConnectRepository())

	ctx := context.Background()
	_ = users.Create(ctx, &user.User{ID: "u1", Username: "alice", PasswordHash: "hashed:pw", Enabled: true})
	raw := "mark-used-fail"
	h := sha256.Sum256([]byte(raw))
	_ = inner.Create(ctx, &user.RefreshToken{
		ID: "rt1", UserID: "u1", TokenHash: hex.EncodeToString(h[:]),
		ExpiresAt: time.Now().Add(time.Hour),
	})

	_, _, err := svc.RefreshToken(ctx, raw, "", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "CommandService.RefreshToken") {
		t.Fatalf("expected wrapped error, got %v", err)
	}
	if !strings.Contains(err.Error(), "mark used boom") {
		t.Fatalf("expected underlying error, got %v", err)
	}
}

// --- generateAndStoreRefreshToken: TTL override applied (line 386-388) ---

func TestGenerateAndStoreRefreshToken_TTLOverrideApplied(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)

	long := 180 * 24 * time.Hour
	before := time.Now()
	_, refresh, err := d.svc.Login(ctx, "alice", "pw", long)
	if err != nil {
		t.Fatal(err)
	}

	// Locate stored RT and check its ExpiresAt is ~ now+long (not default 7d)
	h := sha256.Sum256([]byte(refresh))
	rt, err := d.refreshTkns.GetByTokenHash(ctx, hex.EncodeToString(h[:]))
	if err != nil {
		t.Fatal(err)
	}
	delta := rt.ExpiresAt.Sub(before)
	if delta < long-time.Minute || delta > long+time.Minute {
		t.Fatalf("expected TTL ~ %v, got %v", long, delta)
	}
}

func TestGenerateAndStoreRefreshToken_TTLOverrideZeroUsesDefault(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)

	before := time.Now()
	_, refresh, err := d.svc.Login(ctx, "alice", "pw", 0)
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256([]byte(refresh))
	rt, err := d.refreshTkns.GetByTokenHash(ctx, hex.EncodeToString(h[:]))
	if err != nil {
		t.Fatal(err)
	}
	// default TTL is 7d
	delta := rt.ExpiresAt.Sub(before)
	if delta < 7*24*time.Hour-time.Minute || delta > 7*24*time.Hour+time.Minute {
		t.Fatalf("expected default 7d TTL, got %v", delta)
	}
}

// --- runCleanup: logs warnings on DeleteExpired errors (lines 440-449) ---

func TestRunCleanup_LogsWarningsOnAllRepoErrors(t *testing.T) {
	buf, restore := captureSlogBuffer(t)
	defer restore()

	users := ufakes.NewFakeUserRepository()
	profiles := ufakes.NewFakeProfileRepository()
	innerRT := ufakes.NewFakeRefreshTokenRepository()
	innerRT.Err = errors.New("rt cleanup boom")
	roles := ufakes.NewFakeRoleRepository()
	userRoles := ufakes.NewFakeUserRoleRepository(roles)
	hasher := ufakes.NewFakePasswordHasher()
	tokens := ufakes.NewFakeTokenGenerator()
	events := fakes.NewFakeEventBus()

	innerQC := ufakes.NewFakeQuickConnectRepository()
	qc := &qcRepoDeleteExpiredFail{FakeQuickConnectRepository: innerQC}
	innerReset := ufakes.NewFakePasswordResetTokenRepository()
	reset := &resetRepoDeleteExpiredFail{FakePasswordResetTokenRepository: innerReset}
	cfg := fakes.NewFakeServerConfigRepository()

	svc := NewCommandService(users, profiles, innerRT, roles, userRoles, hasher, tokens, events,
		time.Hour, qc, WithPasswordReset(reset, cfg, false))

	svc.runCleanup(context.Background())

	out := buf.String()
	if !strings.Contains(out, "refresh token cleanup") {
		t.Errorf("expected refresh token cleanup warn, got %q", out)
	}
	if !strings.Contains(out, "quick connect cleanup") {
		t.Errorf("expected quick connect cleanup warn, got %q", out)
	}
	if !strings.Contains(out, "password reset token cleanup") {
		t.Errorf("expected password reset cleanup warn, got %q", out)
	}
}

func TestRunCleanup_NoResetRepo_NoCrash(t *testing.T) {
	d := newAuthTestDeps()
	// resetTokens nil path
	d.svc.runCleanup(context.Background())
}

// --- RequestPasswordReset: success path event publish + deleteByUserID error non-fatal (lines 482-514) ---

func TestRequestPasswordReset_DeleteOldTokensErrorNonFatal(t *testing.T) {
	buf, restore := captureSlogBuffer(t)
	defer restore()

	d := newAuthTestDeps()
	innerReset := ufakes.NewFakePasswordResetTokenRepository()
	cfg := fakes.NewFakeServerConfigRepository()
	_ = cfg.Set(context.Background(), "auth.password_recovery_enabled", "true")

	// Make DeleteByUserID fail
	innerReset.Err = errors.New("delete old boom")
	d.svc = NewCommandService(d.users, d.profiles, d.refreshTkns, d.roles, d.userRoles,
		d.hasher, d.tokens, d.events, time.Hour, ufakes.NewFakeQuickConnectRepository(),
		WithPasswordReset(innerReset, cfg, true))

	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	u, _ := d.users.GetByID(context.Background(), "u1")
	u.Email = "a@a.com"
	_ = d.users.Update(context.Background(), u)

	err := d.svc.RequestPasswordReset(context.Background(), "a@a.com")
	// Since Err on reset repo returns err on Create too, expect error from store
	if err == nil {
		t.Fatal("expected error from store reset token")
	}
	if !strings.Contains(err.Error(), "store reset token") {
		t.Fatalf("expected wrapped store error, got %v", err)
	}
	if !strings.Contains(buf.String(), "cleanup old reset tokens") {
		t.Fatalf("expected cleanup warn log, got %q", buf.String())
	}
}

func TestRequestPasswordReset_EventPublishErrorNonFatal(t *testing.T) {
	d, _, serverCfg := newAuthTestDepsWithReset()
	ctx := context.Background()
	_ = serverCfg.Set(ctx, "auth.password_recovery_enabled", "true")
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	u, _ := d.users.GetByID(ctx, "u1")
	u.Email = "a@a.com"
	_ = d.users.Update(ctx, u)
	d.events.PublishErr = errors.New("bus down")

	if err := d.svc.RequestPasswordReset(ctx, "a@a.com"); err != nil {
		t.Fatalf("expected nil (event err non-fatal), got %v", err)
	}
}

func TestRequestPasswordReset_NoResetTokens(t *testing.T) {
	d := newAuthTestDeps()
	cfg := fakes.NewFakeServerConfigRepository()
	_ = cfg.Set(context.Background(), "auth.password_recovery_enabled", "true")
	d.svc = NewCommandService(d.users, d.profiles, d.refreshTkns, d.roles, d.userRoles,
		d.hasher, d.tokens, d.events, time.Hour, ufakes.NewFakeQuickConnectRepository(),
		WithPasswordReset(nil, cfg, true))
	err := d.svc.RequestPasswordReset(context.Background(), "a@a.com")
	if !errors.Is(err, ErrPasswordRecoveryDisabled) {
		t.Fatalf("expected ErrPasswordRecoveryDisabled, got %v", err)
	}
}

// --- ResetPassword: non-fatal errors on MarkUsed, DeleteByUserID, events (lines 552-562) ---

type resetRepoMarkUsedFail struct {
	*ufakes.FakePasswordResetTokenRepository
}

func (r *resetRepoMarkUsedFail) MarkUsed(_ context.Context, _ string) error {
	return errors.New("mark used reset boom")
}

// TestResetPassword_MarkUsedError_AtomicRollback verifies that a MarkUsed
// failure aborts the operation and rolls back the password update, so the
// reset token cannot be reused with the old password still active OR a
// new password silently committed alongside a still-usable token.
func TestResetPassword_MarkUsedError_AtomicRollback(t *testing.T) {
	d := newAuthTestDeps()
	innerReset := ufakes.NewFakePasswordResetTokenRepository()
	reset := &resetRepoMarkUsedFail{FakePasswordResetTokenRepository: innerReset}
	cfg := fakes.NewFakeServerConfigRepository()
	_ = cfg.Set(context.Background(), "auth.password_recovery_enabled", "true")

	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	raw := "reset-mu"
	h := sha256Hash(raw)
	_ = innerReset.Create(ctx, &user.PasswordResetToken{
		ID: "prt-1", UserID: "u1", TokenHash: h,
		ExpiresAt: time.Now().Add(time.Hour),
	})

	// Snapshot/restore uow: reverts user repo changes when fn errors.
	// Constructed AFTER seeding so the snapshot captures the seeded state.
	uow := newSnapshotUserUOW(ctx, d.users)
	d.svc = NewCommandService(d.users, d.profiles, d.refreshTkns, d.roles, d.userRoles,
		d.hasher, d.tokens, d.events, time.Hour, ufakes.NewFakeQuickConnectRepository(),
		WithPasswordReset(reset, cfg, true), WithUnitOfWork(uow))

	err := d.svc.ResetPassword(ctx, raw, "newpassword123")
	if err == nil {
		t.Fatal("expected error when MarkUsed fails (atomic)")
	}
	if !strings.Contains(err.Error(), "mark reset token used") {
		t.Fatalf("expected wrapped MarkUsed error, got %v", err)
	}
	// Password must NOT be changed (rollback).
	u, _ := d.users.GetByID(ctx, "u1")
	if u.PasswordHash != "hashed:pw" {
		t.Fatalf("password hash should be rolled back, got %q", u.PasswordHash)
	}
	if uow.calls != 1 {
		t.Fatalf("uow.Do should be invoked once, got %d", uow.calls)
	}
}

// snapshotUserUOW captures the user repo state at construction time
// (so it predates any in-place mutation by the service) and restores it
// when fn returns an error, emulating real transactional rollback for
// the in-memory FakeUserRepository (which shares pointers with callers).
type snapshotUserUOW struct {
	users *ufakes.FakeUserRepository
	snap  map[string]user.User
	calls int
}

func newSnapshotUserUOW(ctx context.Context, users *ufakes.FakeUserRepository) *snapshotUserUOW {
	all, _ := users.List(ctx)
	snap := make(map[string]user.User, len(all))
	for _, u := range all {
		snap[string(u.ID)] = *u
	}
	return &snapshotUserUOW{users: users, snap: snap}
}

func (s *snapshotUserUOW) Do(ctx context.Context, fn func(context.Context) error) error {
	s.calls++
	if err := fn(ctx); err != nil {
		for _, u := range s.snap {
			cp := u
			_ = s.users.Update(ctx, &cp)
		}
		return err
	}
	return nil
}

// A refresh-token eviction failure during a password reset must be FATAL and
// roll back the password change: a surviving refresh token can be exchanged for
// a fresh access token (the epoch bump only revokes access tokens), so reporting
// success while the old session lives on would silently bypass the reset.
func TestResetPassword_DeleteRefreshTokensError_FatalRollback(t *testing.T) {
	users := ufakes.NewFakeUserRepository()
	profiles := ufakes.NewFakeProfileRepository()
	innerRT := ufakes.NewFakeRefreshTokenRepository()
	rtRepo := &rtRepoDeleteByUserIDFail{FakeRefreshTokenRepository: innerRT}
	roles := ufakes.NewFakeRoleRepository()
	userRoles := ufakes.NewFakeUserRoleRepository(roles)
	hasher := ufakes.NewFakePasswordHasher()
	tokens := ufakes.NewFakeTokenGenerator()
	events := fakes.NewFakeEventBus()
	reset := ufakes.NewFakePasswordResetTokenRepository()
	cfg := fakes.NewFakeServerConfigRepository()
	_ = cfg.Set(context.Background(), "auth.password_recovery_enabled", "true")

	ctx := context.Background()
	_ = users.Create(ctx, &user.User{ID: "u1", Username: "alice", PasswordHash: "hashed:pw", Enabled: true})
	// Snapshot uow constructed after seeding so a rollback restores the
	// pre-reset password.
	uow := newSnapshotUserUOW(ctx, users)

	svc := NewCommandService(users, profiles, rtRepo, roles, userRoles, hasher, tokens, events,
		time.Hour, ufakes.NewFakeQuickConnectRepository(), WithPasswordReset(reset, cfg, true), WithUnitOfWork(uow))

	raw := "rp-del-rt"
	h := sha256Hash(raw)
	_ = reset.Create(ctx, &user.PasswordResetToken{
		ID: "prt-1", UserID: "u1", TokenHash: h,
		ExpiresAt: time.Now().Add(time.Hour),
	})

	err := svc.ResetPassword(ctx, raw, "newpassword123")
	if err == nil {
		t.Fatal("expected error when refresh-token eviction fails (atomic)")
	}
	if !strings.Contains(err.Error(), "delete refresh tokens") {
		t.Fatalf("expected wrapped delete error, got %v", err)
	}
	u, _ := users.GetByID(ctx, "u1")
	if u.PasswordHash != "hashed:pw" {
		t.Fatalf("password hash should be rolled back, got %q", u.PasswordHash)
	}
}

func TestResetPassword_EventPublishErrorNonFatal(t *testing.T) {
	d, resetTokens, serverCfg := newAuthTestDepsWithReset()
	ctx := context.Background()
	_ = serverCfg.Set(ctx, "auth.password_recovery_enabled", "true")
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)

	raw := "rp-evt"
	h := sha256Hash(raw)
	_ = resetTokens.Create(ctx, &user.PasswordResetToken{
		ID: "prt-1", UserID: "u1", TokenHash: h,
		ExpiresAt: time.Now().Add(time.Hour),
	})
	d.events.PublishErr = errors.New("bus down")

	if err := d.svc.ResetPassword(ctx, raw, "newpassword123"); err != nil {
		t.Fatalf("event publish error should be non-fatal, got %v", err)
	}

	// Confirm password was actually updated (covers success return line 563)
	u, _ := d.users.GetByID(ctx, "u1")
	if u.PasswordHash != "hashed:newpassword123" {
		t.Fatalf("password hash should be updated, got %q", u.PasswordHash)
	}
}

type usersUpdateFail struct {
	*ufakes.FakeUserRepository
}

func (r *usersUpdateFail) Update(_ context.Context, _ *user.User) error {
	return errors.New("update boom")
}

func TestResetPassword_UpdateUserErrorV2(t *testing.T) {
	innerUsers := ufakes.NewFakeUserRepository()
	users := &usersUpdateFail{FakeUserRepository: innerUsers}
	profiles := ufakes.NewFakeProfileRepository()
	rt := ufakes.NewFakeRefreshTokenRepository()
	roles := ufakes.NewFakeRoleRepository()
	userRoles := ufakes.NewFakeUserRoleRepository(roles)
	hasher := ufakes.NewFakePasswordHasher()
	tokens := ufakes.NewFakeTokenGenerator()
	events := fakes.NewFakeEventBus()
	reset := ufakes.NewFakePasswordResetTokenRepository()
	cfg := fakes.NewFakeServerConfigRepository()
	_ = cfg.Set(context.Background(), "auth.password_recovery_enabled", "true")
	svc := NewCommandService(users, profiles, rt, roles, userRoles, hasher, tokens, events,
		time.Hour, ufakes.NewFakeQuickConnectRepository(), WithPasswordReset(reset, cfg, true))

	ctx := context.Background()
	_ = innerUsers.Create(ctx, &user.User{ID: "u1", Username: "alice", PasswordHash: "hashed:pw", Enabled: true})
	raw := "rp-upd2"
	h := sha256Hash(raw)
	_ = reset.Create(ctx, &user.PasswordResetToken{
		ID: "prt-1", UserID: "u1", TokenHash: h,
		ExpiresAt: time.Now().Add(time.Hour),
	})

	err := svc.ResetPassword(ctx, raw, "newpassword123")
	if err == nil {
		t.Fatal("expected error from Update")
	}
	if !strings.Contains(err.Error(), "CommandService.ResetPassword") {
		t.Fatalf("expected wrapped error, got %v", err)
	}
}

// --- loginAttemptLimiter (lines 595, 600, 603, 651, 656, 659) ---

func TestLoginLimiter_LockExpires(t *testing.T) {
	l := newLoginAttemptLimiter(1, 10*time.Millisecond)
	_ = l.Record(context.Background(), "alice", l.maxFails, l.window)
	if !l.isLocked("alice") {
		t.Fatal("expected locked")
	}
	time.Sleep(15 * time.Millisecond)
	if l.isLocked("alice") {
		t.Fatal("expected unlocked after window")
	}
	// isLocked after expiry should have deleted entry
	l.mu.Lock()
	_, exists := l.attempts["alice"]
	l.mu.Unlock()
	if exists {
		t.Fatal("entry should be deleted after lock window expired")
	}
}

func TestLoginLimiter_ResetClearsEntry(t *testing.T) {
	l := newLoginAttemptLimiter(3, time.Minute)
	_ = l.Record(context.Background(), "alice", l.maxFails, l.window)
	l.reset("alice")
	l.mu.Lock()
	_, exists := l.attempts["alice"]
	l.mu.Unlock()
	if exists {
		t.Fatal("reset should delete entry")
	}
}

func TestLoginLimiter_RecordCreatesNewEntry(t *testing.T) {
	l := newLoginAttemptLimiter(5, time.Minute)
	_ = l.Record(context.Background(), "alice", l.maxFails, l.window)
	l.mu.Lock()
	e, ok := l.attempts["alice"]
	l.mu.Unlock()
	if !ok {
		t.Fatal("expected entry created")
	}
	if e.count != 1 {
		t.Fatalf("expected count=1, got %d", e.count)
	}
	_ = l.Record(context.Background(), "alice", l.maxFails, l.window)
	l.mu.Lock()
	e = l.attempts["alice"]
	l.mu.Unlock()
	if e.count != 2 {
		t.Fatalf("expected count=2 (increment not reset), got %d", e.count)
	}
}

func TestLoginLimiter_LockedAtSetAtMaxFails(t *testing.T) {
	l := newLoginAttemptLimiter(2, time.Minute)
	_ = l.Record(context.Background(), "alice", l.maxFails, l.window)
	l.mu.Lock()
	e := l.attempts["alice"]
	locked1 := !e.lockedAt.IsZero()
	l.mu.Unlock()
	if locked1 {
		t.Fatal("should not be locked after 1 attempt (maxFails=2)")
	}
	_ = l.Record(context.Background(), "alice", l.maxFails, l.window)
	l.mu.Lock()
	e = l.attempts["alice"]
	locked2 := !e.lockedAt.IsZero()
	l.mu.Unlock()
	if !locked2 {
		t.Fatal("should be locked after 2 attempts (maxFails=2)")
	}
}

func TestLoginLimiter_Eviction(t *testing.T) {
	l := newLoginAttemptLimiter(5, time.Minute)
	l.maxSize = 3
	_ = l.Record(context.Background(), "a", l.maxFails, l.window)
	_ = l.Record(context.Background(), "b", l.maxFails, l.window)
	_ = l.Record(context.Background(), "c", l.maxFails, l.window)
	_ = l.Record(context.Background(), "d", l.maxFails, l.window) // triggers eviction
	l.mu.Lock()
	size := len(l.attempts)
	l.mu.Unlock()
	if size > 3 {
		t.Fatalf("expected size <= maxSize=3, got %d", size)
	}
}

// --- StartCleanupWithInterval: defer recover catches panic (lines 420-423) ---

type panicRTRepo struct {
	*ufakes.FakeRefreshTokenRepository
	panicked atomic.Bool
}

func (r *panicRTRepo) DeleteExpired(_ context.Context) error {
	if r.panicked.CompareAndSwap(false, true) {
		panic("boom cleanup")
	}
	return nil
}

func TestStartCleanupWithInterval_RecoversFromPanic(t *testing.T) {
	// Don't capture slog here: the recover log happens in a goroutine and
	// would race with slog.SetDefault teardown. Just assert no crash and
	// that goroutine exits cleanly after panic recovery.
	users := ufakes.NewFakeUserRepository()
	profiles := ufakes.NewFakeProfileRepository()
	innerRT := ufakes.NewFakeRefreshTokenRepository()
	rt := &panicRTRepo{FakeRefreshTokenRepository: innerRT}
	roles := ufakes.NewFakeRoleRepository()
	userRoles := ufakes.NewFakeUserRoleRepository(roles)
	hasher := ufakes.NewFakePasswordHasher()
	tokens := ufakes.NewFakeTokenGenerator()
	events := fakes.NewFakeEventBus()
	svc := NewCommandService(users, profiles, rt, roles, userRoles, hasher, tokens, events,
		time.Hour, ufakes.NewFakeQuickConnectRepository())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Should not panic out of goroutine thanks to defer recover
	svc.StartCleanupWithInterval(ctx, time.Hour)
	time.Sleep(50 * time.Millisecond)
	if !rt.panicked.Load() {
		t.Fatal("expected DeleteExpired called (and panicked)")
	}
}

// --- Setup: profile publish event error non-fatal (line 143-145) ---

func TestSetup_ProfileEventPublishError_StillReturnsUser(t *testing.T) {
	buf, restore := captureSlogBuffer(t)
	defer restore()

	d := newAuthTestDeps()
	d.events.PublishErr = errors.New("bus boom")
	u, err := d.svc.Setup(context.Background(), "admin", "longpassword", "a@a.com")
	if err != nil {
		t.Fatal(err)
	}
	if u == nil || u.Username != "admin" {
		t.Fatal("expected user returned despite event errors")
	}
	// Both user.created and user.profile.created publishes should log warnings
	if !strings.Contains(buf.String(), "publish event") {
		t.Fatalf("expected publish event warn, got %q", buf.String())
	}
}

// silence import if needed
var _ = shared.ErrNotFound

// A failed profile lookup must NOT mint a token with an empty ProfileID. An
// empty profile is read as "no parental restriction" by every gate in the
// server (parental.go's checker and the gRPC filter resolver both short-circuit
// on it), so swallowing the error here turns a transient DB fault into a silent,
// complete parental bypass lasting the whole access-token TTL.
//
// Fail closed: refusing the refresh logs the user out, which is recoverable.
// Minting an unrestricted token is not.
func TestRefreshToken_ProfileLookupFails_RefusesRatherThanDroppingProfile(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true) // non-admin: admins are parental-exempt by design
	kid := shared.ProfileID("p-kid")
	_ = d.profiles.Create(ctx, &user.Profile{ID: kid, UserID: "u1", Name: "Kid", IsKids: true, IsDefault: true})

	raw := "refresh-proflookup-tok"
	h := sha256.Sum256([]byte(raw))
	_ = d.refreshTkns.Create(ctx, &user.RefreshToken{
		ID: "rt-pl", UserID: "u1", TokenHash: hex.EncodeToString(h[:]),
		ExpiresAt: time.Now().Add(time.Hour), ProfileID: &kid,
	})

	d.profiles.Err = errors.New("connection reset by peer")

	access, _, err := d.svc.RefreshToken(ctx, raw, "", "")
	if err == nil {
		claims, _ := d.tokens.ValidateAccessToken(access)
		t.Fatalf("a failed profile lookup must refuse the refresh, got a token with ProfileID=%q", claims.ProfileID)
	}
	t.Logf("refused with: %v", err)
}

// stubLoginAttempts records PurgeStale calls.
type stubLoginAttempts struct {
	purged chan time.Duration
}

func (s *stubLoginAttempts) IsLocked(context.Context, string) (bool, error) { return false, nil }
func (s *stubLoginAttempts) Record(context.Context, string, int, time.Duration) error {
	return nil
}
func (s *stubLoginAttempts) Reset(context.Context, string) error { return nil }
func (s *stubLoginAttempts) PurgeStale(_ context.Context, staleFor time.Duration) (int64, error) {
	select {
	case s.purged <- staleFor:
	default:
	}
	return 3, nil
}

// login_attempts takes UNAUTHENTICATED writes on purpose: the unknown-user path
// records an attempt so a missing account costs the same as a wrong password.
// The price is one permanent row per distinct username an attacker tries, so
// without this sweep a credential-stuffing dictionary is stored forever. The
// in-memory limiter this table replaced had both a TTL and a size cap.
func TestStartLoginAttemptCleanup_SweepsOnStart(t *testing.T) {
	d := newAuthTestDeps()
	stub := &stubLoginAttempts{purged: make(chan time.Duration, 4)}
	WithLoginAttemptRepository(stub)(d.svc)

	d.svc.StartLoginAttemptCleanup(t.Context())

	select {
	case staleFor := <-stub.purged:
		// Must be comfortably past any rate-limit window, so a row is only
		// dropped once it can no longer influence a decision.
		if staleFor < time.Hour {
			t.Fatalf("stale threshold %v is too aggressive - it could drop a row still inside its window", staleFor)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the cleanup must sweep immediately on start, not only after the first tick")
	}
}

// An unwired repository must not spawn a goroutine that nil-derefs.
func TestStartLoginAttemptCleanup_NilRepoIsSafe(t *testing.T) {
	svc := &CommandService{}
	svc.StartLoginAttemptCleanup(t.Context())
	time.Sleep(100 * time.Millisecond)
}

// --- Profile last_active_at: which profile was used, and when ---

func TestLogin_StampsOnlyTheActivatedProfile(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice", IsDefault: true})
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p2", UserID: "u1", Name: "Kids"})

	if _, _, err := d.svc.Login(ctx, "alice", "pw"); err != nil {
		t.Fatal(err)
	}
	p1, _ := d.profiles.GetByID(ctx, "p1")
	if p1.LastActiveAt == nil {
		t.Fatal("login lands on the default profile: it must be stamped")
	}
	p2, _ := d.profiles.GetByID(ctx, "p2")
	if p2.LastActiveAt != nil {
		t.Fatal("a profile nobody activated must stay unstamped")
	}
}

func TestRefreshToken_ProfileSwitchStampsTheTargetProfile(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alice", IsDefault: true})
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p2", UserID: "u1", Name: "Kids"})

	raw := "refresh-switch-tok"
	h := sha256.Sum256([]byte(raw))
	_ = d.refreshTkns.Create(ctx, &user.RefreshToken{
		ID: "rt1", UserID: "u1", TokenHash: hex.EncodeToString(h[:]),
		ExpiresAt: time.Now().Add(time.Hour),
	})

	before := time.Now().Add(-time.Second)
	if _, _, err := d.svc.RefreshToken(ctx, raw, "p2", ""); err != nil {
		t.Fatal(err)
	}
	p2, _ := d.profiles.GetByID(ctx, "p2")
	if p2.LastActiveAt == nil || p2.LastActiveAt.Before(before) {
		t.Fatalf("switched-to profile must be stamped now, got %v", p2.LastActiveAt)
	}
	p1, _ := d.profiles.GetByID(ctx, "p1")
	if p1.LastActiveAt != nil {
		t.Fatal("the profile being left must not be stamped by the switch")
	}
}

// An admin looking around under someone's profile is not that profile being
// used: "last active" would otherwise lie to the very person reading it.
func TestImpersonate_DoesNotStampProfileLastActive(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "usr-admin", "root", "pw", true, true)
	seedAuthUser(t, d, "usr-bob", "bob", "pw", false, true)
	_ = d.profiles.Create(ctx, &user.Profile{ID: "prf-bob-1", UserID: "usr-bob", Name: "Bob", IsDefault: true})

	if _, err := d.svc.Impersonate(ctx, "usr-admin", "usr-bob", "prf-bob-1"); err != nil {
		t.Fatal(err)
	}
	p, _ := d.profiles.GetByID(ctx, "prf-bob-1")
	if p.LastActiveAt != nil {
		t.Fatalf("impersonation stamped the target profile: %v", p.LastActiveAt)
	}
}
