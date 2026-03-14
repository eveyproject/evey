package notification

import "time"

type NotificationCreated struct {
	NotificationID string
	ProfileID      string
	Type           Type
	Title          string
	MediaItemID    string
	OccurredAt     time.Time
}

func (e NotificationCreated) EventName() string { return "notification.created" }

type NotificationRead struct {
	NotificationID string
	ProfileID      string
	OccurredAt     time.Time
}

func (e NotificationRead) EventName() string { return "notification.read" }

type NotificationDismissed struct {
	NotificationID string
	ProfileID      string
	OccurredAt     time.Time
}

func (e NotificationDismissed) EventName() string { return "notification.dismissed" }
