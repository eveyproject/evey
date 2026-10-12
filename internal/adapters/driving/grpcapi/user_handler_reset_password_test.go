package grpcapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	appuser "github.com/eveyproject/evey/internal/app/user"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	"github.com/eveyproject/evey/internal/domain/user"
	userfakes "github.com/eveyproject/evey/internal/domain/user/fakes"
	"google.golang.org/grpc/codes"
)

// setupUserHandlerWithReset wires a full CommandService with PasswordReset enabled
// so RequestPasswordReset and ResetPassword flows exercise the real code path.
func setupUserHandlerWithReset(t *testing.T) (*userHandler, *userfakes.FakeUserRepository, *userfakes.FakePasswordResetTokenRepository, *userfakes.FakePasswordHasher) {
	t.Helper()
	userRepo := userfakes.NewFakeUserRepository()
	profileRepo := userfakes.NewFakeProfileRepository()
	refreshRepo := userfakes.NewFakeRefreshTokenRepository()
	roleRepo := userfakes.NewFakeRoleRepository()
	userRoleRepo := userfakes.NewFakeUserRoleRepository(roleRepo)
	hasher := userfakes.NewFakePasswordHasher()
	tokenGen := userfakes.NewFakeTokenGenerator()
	events := sharedfakes.NewFakeEventBus()
	resetRepo := userfakes.NewFakePasswordResetTokenRepository()
	cfg := sharedfakes.NewFakeServerConfigRepository()

	cmd := appuser.NewCommandService(
		userRepo, profileRepo, refreshRepo,
		roleRepo, userRoleRepo,
		hasher, tokenGen, events,
		7*24*time.Hour, userfakes.NewFakeQuickConnectRepository(),
		appuser.WithPasswordReset(resetRepo, cfg, true),
	)

	h := &userHandler{auth: cmd}
	return h, userRepo, resetRepo, hasher
}

func TestRequestPasswordReset_AlwaysReturnsEmptyResponse(t *testing.T) {
	h, userRepo, _, hasher := setupUserHandlerWithReset(t)
	hashed, _ := hasher.Hash("secretpw1")
	_ = userRepo.Create(context.Background(), &user.User{
		ID: "usr-1", Username: "alice", Email: "a@example.com",
		PasswordHash: hashed, Enabled: true,
	})

	resp, err := h.RequestPasswordReset(context.Background(), &eveyv1.RequestPasswordResetRequest{Email: "a@example.com"})
	if err != nil {
		t.Fatalf("RequestPasswordReset: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

func TestRequestPasswordReset_UnknownEmail_StillOK_NoLeak(t *testing.T) {
	h, _, _, _ := setupUserHandlerWithReset(t)
	// No user seeded; handler must not leak via error code.
	resp, err := h.RequestPasswordReset(context.Background(), &eveyv1.RequestPasswordResetRequest{Email: "nobody@example.com"})
	if err != nil {
		t.Fatalf("RequestPasswordReset: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

func TestResetPassword_InvalidToken(t *testing.T) {
	h, _, _, _ := setupUserHandlerWithReset(t)
	_, err := h.ResetPassword(context.Background(), &eveyv1.ResetPasswordRequest{
		Token: "bogus-token", NewPassword: "longenough12",
	})
	assertCode(t, err, codes.InvalidArgument)
}

func TestResetPassword_TooShort(t *testing.T) {
	h, userRepo, resetRepo, hasher := setupUserHandlerWithReset(t)
	hashed, _ := hasher.Hash("secretpw1")
	u := &user.User{
		ID: "usr-2", Username: "bob", Email: "b@example.com",
		PasswordHash: hashed, Enabled: true,
	}
	_ = userRepo.Create(context.Background(), u)

	rawTok := "raw-reset-token-xyz"
	h256 := sha256.Sum256([]byte(rawTok))
	tokenHash := hex.EncodeToString(h256[:])
	_ = resetRepo.Create(context.Background(), &user.PasswordResetToken{
		ID: "prt-1", UserID: u.ID, TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(30 * time.Minute),
		CreatedAt: time.Now(),
	})

	_, err := h.ResetPassword(context.Background(), &eveyv1.ResetPasswordRequest{
		Token: rawTok, NewPassword: "short",
	})
	assertCode(t, err, codes.InvalidArgument)
}

func TestResetPassword_HappyPath(t *testing.T) {
	h, userRepo, resetRepo, hasher := setupUserHandlerWithReset(t)
	hashed, _ := hasher.Hash("oldpassword1")
	u := &user.User{
		ID: "usr-3", Username: "carol", Email: "c@example.com",
		PasswordHash: hashed, Enabled: true,
	}
	_ = userRepo.Create(context.Background(), u)

	rawTok := "another-reset-token-abc"
	h256 := sha256.Sum256([]byte(rawTok))
	tokenHash := hex.EncodeToString(h256[:])
	_ = resetRepo.Create(context.Background(), &user.PasswordResetToken{
		ID: "prt-2", UserID: u.ID, TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(30 * time.Minute),
		CreatedAt: time.Now(),
	})

	resp, err := h.ResetPassword(context.Background(), &eveyv1.ResetPasswordRequest{
		Token: rawTok, NewPassword: "newpassword12",
	})
	if err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	// User's password should now verify against the new password.
	got, err := userRepo.GetByID(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	ok, _ := hasher.Verify("newpassword12", got.PasswordHash)
	if !ok {
		t.Error("new password does not verify against stored hash")
	}
}
