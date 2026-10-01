package user

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/id"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// password_reset.go holds the email-based password recovery flow: the enable
// gate (operator setting + SMTP), the reset-request path (opaque to whether the
// email exists) and the atomic reset that also evicts every refresh token and
// bumps the token epoch.

// PasswordRecoveryConfigKey is the server setting the admin panel's
// password-recovery switch writes; the reset flow reads it on every request.
const PasswordRecoveryConfigKey = "auth.password_recovery_enabled"

func (s *CommandService) IsPasswordRecoveryEnabled(ctx context.Context) bool {
	if s.smtpConfigured == nil || !s.smtpConfigured() || s.serverCfg == nil {
		return false
	}
	entry, err := s.serverCfg.Get(ctx, PasswordRecoveryConfigKey)
	if err != nil {
		return false
	}
	return entry.Value == "true"
}

const (
	resetTokenBytes       = 32
	passwordResetTokenTTL = 30 * time.Minute
)

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
		slog.WarnContext(ctx, "cleanup old reset tokens", slog.String("userID", string(u.ID)), slog.Any("err", err))
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
		slog.WarnContext(ctx, "publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
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

	newPassword = normalizePassword(newPassword)
	if len(newPassword) < s.minPasswordLength {
		return ErrPasswordTooShort
	}

	u, err := s.users.GetByID(ctx, t.UserID)
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
			slog.WarnContext(ctx, "revoke access tokens after password reset", slog.Any("err", err))
		}
	}

	evt := user.PasswordChanged{UserID: u.ID, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evt); err != nil {
		slog.WarnContext(ctx, "publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
	}
	return nil
}
