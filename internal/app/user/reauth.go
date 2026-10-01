package user

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/evey-project/evey/internal/domain/shared"
)

// ErrNoLocalPassword: the account signs in through SSO only, so it has no
// password to re-enter; an admin or the e-mail reset gives it one first.
var ErrNoLocalPassword = errors.New("account has no local password")

// VerifyCurrentPassword re-authenticates userID before a credential change.
// Same lockout counter and normalisation as Login: a stolen token cannot
// brute-force the password here any faster than at the login form.
func (s *CommandService) VerifyCurrentPassword(ctx context.Context, userID, password string) error {
	u, err := s.users.GetByID(ctx, shared.UserID(userID))
	if err != nil {
		return fmt.Errorf("CommandService.VerifyCurrentPassword: %w", err)
	}
	if u.PasswordHash == "" {
		return ErrNoLocalPassword
	}
	// Same keys as Login: the hard lock per (client IP, user), the per-user cap.
	pairKey, userKey := loginAttemptKeys(ctx, u.Username)
	if s.loginLocked(ctx, u.Username, pairKey, userKey) {
		return ErrAccountLocked
	}
	ok, vErr := s.hasher.Verify(normalizePassword(password), u.PasswordHash)
	if vErr != nil || !ok {
		s.recordLoginFailure(ctx, u.Username, pairKey, userKey)
		slog.WarnContext(ctx, "reauth failed", slog.String("userID", userID), slog.String("reason", "bad_password"))
		return ErrInvalidCredentials
	}
	if resetErr := s.loginAttempts.Reset(ctx, pairKey); resetErr != nil {
		slog.WarnContext(ctx, "reauth attempt Reset", slog.String("userID", userID), slog.Any("err", resetErr))
	}
	return nil
}
