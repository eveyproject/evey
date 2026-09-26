package user

import (
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
)

type UserCreated struct {
	UserID     shared.UserID
	Username   string
	IsAdmin    bool
	OccurredAt time.Time
}

func (e UserCreated) EventName() string { return "user.created" }

type UserDeleted struct {
	UserID     shared.UserID
	OccurredAt time.Time
}

func (e UserDeleted) EventName() string { return "user.deleted" }

type UserDisabled struct {
	UserID     shared.UserID
	OccurredAt time.Time
}

func (e UserDisabled) EventName() string { return "user.disabled" }

type UserEnabled struct {
	UserID     shared.UserID
	OccurredAt time.Time
}

func (e UserEnabled) EventName() string { return "user.enabled" }

type ProfileCreated struct {
	ProfileID  shared.ProfileID
	UserID     shared.UserID
	Name       string
	OccurredAt time.Time
}

func (e ProfileCreated) EventName() string { return "user.profile.created" }

type LoginSucceeded struct {
	UserID     shared.UserID
	ClientIP   string
	AuthMethod string
	OccurredAt time.Time
}

func (e LoginSucceeded) EventName() string { return "user.login.succeeded" }

// LoginFailed is a refused password login. Username is what was typed (never
// the password); Reason is unknown_user, bad_password, disabled or locked, so
// a receiver can tell a replayed list from a brute force on one account.
type LoginFailed struct {
	Username   string
	Reason     string
	AuthMethod string
	OccurredAt time.Time
}

func (e LoginFailed) EventName() string { return "user.login.failed" }

type PasswordChanged struct {
	UserID     shared.UserID
	OccurredAt time.Time
}

func (e PasswordChanged) EventName() string { return "user.password.changed" }

type ProfileDeleted struct {
	ProfileID  shared.ProfileID
	UserID     shared.UserID
	OccurredAt time.Time
}

func (e ProfileDeleted) EventName() string { return "user.profile.deleted" }

type APIKeyCreated struct {
	KeyID      string
	UserID     shared.UserID
	Name       string
	OccurredAt time.Time
}

func (e APIKeyCreated) EventName() string { return "user.apikey.created" }

type APIKeyRevoked struct {
	KeyID      string
	OccurredAt time.Time
}

func (e APIKeyRevoked) EventName() string { return "user.apikey.revoked" }

type PasswordResetRequested struct {
	UserID     shared.UserID
	Email      string
	ResetToken string // raw token for email sending
	ExpiresAt  time.Time
	OccurredAt time.Time
}

func (e PasswordResetRequested) EventName() string { return "user.password.reset_requested" }
