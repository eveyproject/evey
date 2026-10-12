package mediarequest

import (
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
)

// RequestCreated is published when a user submits a new media request. The
// notification service subscribes to it to page the operators (admin-only).
type RequestCreated struct {
	RequestID  string
	ProfileID  shared.ProfileID // requester
	UserID     string
	MediaType  MediaType
	Title      string
	OccurredAt time.Time
}

// EventName implements shared.Event.
func (RequestCreated) EventName() string { return "media.request.created" }

// RequestAnswered is published when an operator answers a request. The
// notification service subscribes to it to notify the requester.
type RequestAnswered struct {
	RequestID  string
	ProfileID  shared.ProfileID // requester (the notification target)
	Title      string
	Status     Status
	Note       string
	OccurredAt time.Time
}

// EventName implements shared.Event.
func (RequestAnswered) EventName() string { return "media.request.answered" }

var (
	_ shared.Event = RequestCreated{}
	_ shared.Event = RequestAnswered{}
)
