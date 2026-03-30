package email

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/gravity-zero/evey/internal/domain/shared"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
)

// PasswordResetEmailer subscribes to user.password.reset_requested and sends reset link emails.
type PasswordResetEmailer struct {
	sender  Sender
	baseURL string
}

func NewPasswordResetEmailer(sender Sender, baseURL string) *PasswordResetEmailer {
	return &PasswordResetEmailer{sender: sender, baseURL: baseURL}
}

func (e *PasswordResetEmailer) Register(bus shared.EventBus) {
	bus.Subscribe("user.password.reset_requested", e.handle)
}

func (e *PasswordResetEmailer) handle(_ context.Context, ev shared.Event) {
	evt, ok := ev.(domainuser.PasswordResetRequested)
	if !ok {
		return
	}

	resetURL := fmt.Sprintf("%s/reset-password?token=%s", e.baseURL, evt.ResetToken)
	subject := "Evey — Password Reset"
	body := fmt.Sprintf("A password reset was requested for your account.\n\nClick the link below to reset your password:\n%s\n\nThis link expires in 30 minutes.\n\nIf you did not request this, ignore this email.", resetURL)

	if err := e.sender.Send([]string{evt.Email}, subject, body); err != nil {
		slog.Warn("email: send password reset", slog.String("to", evt.Email), slog.Any("err", err))
	}
}
