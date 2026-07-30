package user

import (
	"context"
	"log/slog"
	"time"
)

// cleanup.go holds the background sweep loops owned by CommandService: expired
// refresh / quick-connect / password-reset tokens (hourly by default) and the
// login-attempt table (hourly, because it takes unauthenticated writes).

const defaultCleanupInterval = 1 * time.Hour

func (s *CommandService) StartCleanup(ctx context.Context) {
	s.StartCleanupWithInterval(ctx, defaultCleanupInterval)
}

func (s *CommandService) StartCleanupWithInterval(ctx context.Context, interval time.Duration) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.ErrorContext(ctx, "panic in user cleanup", slog.Any("recover", r))
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
		slog.WarnContext(ctx, "refresh token cleanup", slog.Any("err", err))
	}
	if err := s.quickConnect.DeleteExpired(ctx); err != nil {
		slog.WarnContext(ctx, "quick connect cleanup", slog.Any("err", err))
	}
	if s.resetTokens != nil {
		if err := s.resetTokens.DeleteExpired(ctx); err != nil {
			slog.WarnContext(ctx, "password reset token cleanup", slog.Any("err", err))
		}
	}
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
				slog.ErrorContext(ctx, "login-attempt cleanup: panic recovered", slog.Any("panic", r))
			}
		}()
		sweep := func() {
			n, err := s.loginAttempts.PurgeStale(ctx, loginAttemptStaleAfter)
			if err != nil {
				slog.WarnContext(ctx, "login-attempt cleanup failed", slog.Any("err", err))
				return
			}
			if n > 0 {
				slog.InfoContext(ctx, "login-attempt cleanup", slog.Int64("removed", n))
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
