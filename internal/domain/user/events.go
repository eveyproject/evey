package user

import "time"

type UserCreated struct {
	UserID     string
	Username   string
	IsAdmin    bool
	OccurredAt time.Time
}

func (e UserCreated) EventName() string { return "user.created" }

type UserDeleted struct {
	UserID     string
	OccurredAt time.Time
}

func (e UserDeleted) EventName() string { return "user.deleted" }

type UserDisabled struct {
	UserID     string
	OccurredAt time.Time
}

func (e UserDisabled) EventName() string { return "user.disabled" }

type ProfileCreated struct {
	ProfileID  string
	UserID     string
	Name       string
	OccurredAt time.Time
}

func (e ProfileCreated) EventName() string { return "user.profile.created" }

type LoginSucceeded struct {
	UserID     string
	ClientIP   string
	AuthMethod string
	OccurredAt time.Time
}

func (e LoginSucceeded) EventName() string { return "user.login.succeeded" }

type LoginFailed struct {
	Username   string
	ClientIP   string
	Reason     string
	OccurredAt time.Time
}

func (e LoginFailed) EventName() string { return "user.login.failed" }
