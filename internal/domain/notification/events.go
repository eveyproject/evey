package notification

import (
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
)

type NotificationCreated struct {
	NotificationID string
	ProfileID      shared.ProfileID
	Type           Type
	Title          string
	MediaItemID    shared.MediaItemID
	OccurredAt     time.Time
}

func (e NotificationCreated) EventName() string { return "notification.created" }

type NotificationRead struct {
	NotificationID string
	ProfileID      shared.ProfileID
	OccurredAt     time.Time
}

func (e NotificationRead) EventName() string { return "notification.read" }

type NotificationDismissed struct {
	NotificationID string
	ProfileID      shared.ProfileID
	OccurredAt     time.Time
}

func (e NotificationDismissed) EventName() string { return "notification.dismissed" }
