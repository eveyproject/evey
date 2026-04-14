package activity

import (
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
)

type LoginRecorded struct {
	UserID     shared.UserID
	ClientIP   string
	AuthMethod string
	Success    bool
	OccurredAt time.Time
}

func (e LoginRecorded) EventName() string { return "activity.login.recorded" }

type MetricRecorded struct {
	MetricID   string
	OccurredAt time.Time
}

func (e MetricRecorded) EventName() string { return "activity.metric.recorded" }

type AuditLogCreated struct {
	EntryID    string
	UserID     shared.UserID
	Action     string
	OccurredAt time.Time
}

func (e AuditLogCreated) EventName() string { return "activity.audit.created" }
