package activity

import (
	"context"
	"errors"
	"log/slog"

	"github.com/eveyproject/evey/internal/domain/activity"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
	"github.com/google/uuid"
)

// UserLookup resolves the account a refused login named: the failure event
// carries the username typed, the history row needs the user id.
type UserLookup interface {
	GetByUsername(ctx context.Context, username string) (*user.User, error)
}

// LoginRecorder writes the login history from the login events. The history
// table and its query existed with no writer: the events were published for
// the webhooks only, so the admin's login history stayed empty.
type LoginRecorder struct {
	cmd   *CommandService
	users UserLookup
}

func NewLoginRecorder(cmd *CommandService, users UserLookup) *LoginRecorder {
	return &LoginRecorder{cmd: cmd, users: users}
}

func (r *LoginRecorder) Register(bus shared.EventBus) {
	bus.Subscribe("user.login.succeeded", r.handleSucceeded)
	bus.Subscribe("user.login.failed", r.handleFailed)
}

func (r *LoginRecorder) handleSucceeded(ctx context.Context, ev shared.Event) {
	evt, ok := ev.(user.LoginSucceeded)
	if !ok {
		return
	}
	r.record(ctx, &activity.LoginEntry{
		ID:         uuid.New().String(),
		UserID:     evt.UserID,
		ClientIP:   evt.ClientIP,
		AuthMethod: evt.AuthMethod,
		Success:    true,
	})
}

// handleFailed books a refusal against the account it named. A username
// that is no account (unknown_user) has no row to hang it on and is not
// recorded: the history is the account's, not the attacker's.
func (r *LoginRecorder) handleFailed(ctx context.Context, ev shared.Event) {
	evt, ok := ev.(user.LoginFailed)
	if !ok || r.users == nil || evt.Username == "" {
		return
	}
	u, err := r.users.GetByUsername(ctx, evt.Username)
	if err != nil {
		if !errors.Is(err, shared.ErrNotFound) {
			slog.WarnContext(ctx, "login history: resolve user", slog.Any("err", err))
		}
		return
	}
	reason := evt.Reason
	r.record(ctx, &activity.LoginEntry{
		ID:            uuid.New().String(),
		UserID:        u.ID,
		ClientIP:      evt.ClientIP,
		AuthMethod:    evt.AuthMethod,
		Success:       false,
		FailureReason: &reason,
	})
}

func (r *LoginRecorder) record(ctx context.Context, entry *activity.LoginEntry) {
	if err := r.cmd.RecordLogin(ctx, entry); err != nil {
		slog.WarnContext(ctx, "login history: record", slog.String("user_id", string(entry.UserID)), slog.Any("err", err))
	}
}
