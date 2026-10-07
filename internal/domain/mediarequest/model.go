// Package mediarequest is the domain for user-submitted media requests: a
// user asks the operator to add a movie/series/album, the operator moderates
// (approve / reject / later) with a note, and the requester is notified.
package mediarequest

import (
	"errors"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
)

// MediaType is the kind of media a request targets.
type MediaType string

const (
	MediaTypeMovie MediaType = "movie"
	MediaTypeTV    MediaType = "tv"
	MediaTypeMusic MediaType = "music"
)

// IsValid reports whether the media type is one Evey understands.
func (m MediaType) IsValid() bool {
	switch m {
	case MediaTypeMovie, MediaTypeTV, MediaTypeMusic:
		return true
	}
	return false
}

// Status is the moderation state of a request.
type Status string

const (
	// StatusPending is the initial state: awaiting an operator decision.
	StatusPending Status = "pending"
	// StatusApproved means the operator accepted the request.
	StatusApproved Status = "approved"
	// StatusRejected means the operator declined it.
	StatusRejected Status = "rejected"
	// StatusLater means the operator deferred it (a soft "not now").
	StatusLater Status = "later"
)

// IsValid reports whether the status is a known moderation state.
func (s Status) IsValid() bool {
	switch s {
	case StatusPending, StatusApproved, StatusRejected, StatusLater:
		return true
	}
	return false
}

// IsAnswer reports whether the status is an operator decision (i.e. not the
// initial pending state). Answering a request transitions it to one of these.
func (s Status) IsAnswer() bool {
	switch s {
	case StatusApproved, StatusRejected, StatusLater:
		return true
	}
	return false
}

// Domain errors.
var (
	ErrNotFound  = errors.New("media request not found")
	ErrNotOwner  = errors.New("media request not owned by caller")
	ErrDisabled  = errors.New("media requests are disabled for this user")
	ErrDuplicate = errors.New("an open request already exists for this title")
	// ErrAlreadyHeld guards the pointless request: the server already holds this
	// exact title. Enforced here, not just greyed out in the UI - the client is
	// not the authority on what the library contains.
	ErrAlreadyHeld = errors.New("this title is already in the library")
)

// Request is a single user-submitted media request.
type Request struct {
	ID        string
	ProfileID shared.ProfileID // requester's profile (who asked, and who gets the answer notification)
	UserID    string           // requester's account (drives per-user access checks)
	MediaType MediaType

	// Provider identifies the metadata source the requester picked the title
	// from (tmdb / imdb / musicbrainz). ProviderID is that source's id. Both
	// may be empty for a free-text request that resolved to no external match.
	Provider   string
	ProviderID string

	Title     string
	Year      *int
	PosterURL string
	// Overview, ReleaseDate and Rating are what the requester saw when picking
	// the title, kept so the operator can judge the request without opening a
	// browser. Rating is on a 0-10 scale; all three are optional.
	Overview    string
	ReleaseDate *time.Time
	Rating      *float64

	Status    Status
	AdminNote string // operator's handwritten note, surfaced to the requester
	// AnsweredBy is the profile of the operator who answered; empty while pending.
	AnsweredBy shared.ProfileID

	CreatedAt  time.Time
	UpdatedAt  time.Time
	AnsweredAt *time.Time
}

// Validate enforces the aggregate invariants.
func (r *Request) Validate() error {
	if r.ID == "" {
		return errors.New("media request requires an ID")
	}
	if r.ProfileID == "" {
		return errors.New("media request requires a profile ID")
	}
	if r.UserID == "" {
		return errors.New("media request requires a user ID")
	}
	if !r.MediaType.IsValid() {
		return errors.New("media request requires a valid media type")
	}
	if r.Title == "" {
		return errors.New("media request requires a title")
	}
	if !r.Status.IsValid() {
		return errors.New("media request has an invalid status")
	}
	return nil
}

// Option customises a Request at construction.
type Option func(*Request)

// WithYear sets the release year.
func WithYear(year int) Option {
	return func(r *Request) { r.Year = &year }
}

// WithPosterURL sets the poster URL the requester saw when picking the title.
func WithPosterURL(url string) Option {
	return func(r *Request) { r.PosterURL = url }
}

// WithOverview sets the synopsis the requester saw.
func WithOverview(overview string) Option {
	return func(r *Request) { r.Overview = overview }
}

// WithReleaseDate sets the provider's premiere date.
func WithReleaseDate(d time.Time) Option {
	return func(r *Request) { r.ReleaseDate = &d }
}

// WithRating sets the community rating (0-10).
func WithRating(rating float64) Option {
	return func(r *Request) { r.Rating = &rating }
}

// WithProvider records the metadata source and its id for the picked title.
func WithProvider(provider, providerID string) Option {
	return func(r *Request) {
		r.Provider = provider
		r.ProviderID = providerID
	}
}

// NewRequest is the canonical constructor. Required identity fields (id,
// profile, user, media type, title) are positional so construction never
// succeeds with them missing; everything else is an option. Stamps timestamps,
// defaults the status to pending, and validates.
func NewRequest(id string, profileID shared.ProfileID, userID string, mediaType MediaType, title string, opts ...Option) (*Request, error) {
	now := time.Now()
	r := &Request{
		ID:        id,
		ProfileID: profileID,
		UserID:    userID,
		MediaType: mediaType,
		Title:     title,
		Status:    StatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
	for _, opt := range opts {
		opt(r)
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return r, nil
}

// Answer transitions a pending request to an operator decision, attaching the
// note and the answering operator's profile. Returns an error if the status is
// not an answer state.
func (r *Request) Answer(status Status, note string, answeredBy shared.ProfileID) error {
	if !status.IsAnswer() {
		return errors.New("answer requires approved, rejected or later")
	}
	now := time.Now()
	r.Status = status
	r.AdminNote = note
	r.AnsweredBy = answeredBy
	r.AnsweredAt = &now
	r.UpdatedAt = now
	return r.Validate()
}
