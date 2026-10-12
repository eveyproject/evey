package notification

import (
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
)

type NotificationCreated struct {
	NotificationID string
	ProfileID      shared.ProfileID
	Type           Type
	Title          string
	MediaItemID    shared.MediaItemID
	OccurredAt     time.Time
	// FanoutID is shared by every per-profile row of one fan-out, so a
	// per-account consumer (e-mail) acts once per account, not per profile.
	FanoutID string
	// TitleKey/TitleParams/TitleFallback are the push title, locale-agnostic:
	// Title is the stored internal label ("new_movie:X"), never user-facing.
	TitleKey      string
	TitleParams   map[string]string
	TitleFallback string
	Message       string
}

func (e NotificationCreated) EventName() string { return "notification.created" }
