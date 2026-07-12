package mediarequest

import "context"

// Repository persists media requests. Query and command methods live on one
// interface: the aggregate is small and always accessed by a single owner.
type Repository interface {
	Create(ctx context.Context, r *Request) error
	// GetByID returns ErrNotFound when the id is unknown.
	GetByID(ctx context.Context, id string) (*Request, error)
	Update(ctx context.Context, r *Request) error
	Delete(ctx context.Context, id string) error

	// ListByProfile returns the requests a given profile submitted, newest first.
	ListByProfile(ctx context.Context, profileID string, limit, offset int) ([]*Request, error)
	// ListAll returns every request (operator moderation view), newest first.
	// When statuses is non-empty the result is filtered to those states.
	ListAll(ctx context.Context, statuses []Status, limit, offset int) ([]*Request, error)
	// HasOpenRequest reports whether the profile already has a non-rejected
	// request for the same (mediaType, providerID) - used to reject duplicates.
	HasOpenRequest(ctx context.Context, profileID string, mediaType MediaType, providerID string) (bool, error)
}

// Access decides whether a given user may submit requests. The policy is a
// global default (operator-set) overridable per user.
type Access interface {
	// GlobalDefault returns whether requests are enabled by default for users
	// without an explicit override.
	GlobalDefault(ctx context.Context) (bool, error)
	SetGlobalDefault(ctx context.Context, enabled bool) error
	// Override returns (value, true) when the user has an explicit setting, or
	// (_, false) when they fall back to the global default.
	Override(ctx context.Context, userID string) (bool, bool, error)
	// SetOverride sets an explicit per-user value.
	SetOverride(ctx context.Context, userID string, enabled bool) error
	// ClearOverride removes the per-user setting (falls back to the default).
	ClearOverride(ctx context.Context, userID string) error
	// ListOverrides returns all explicit per-user settings.
	ListOverrides(ctx context.Context) (map[string]bool, error)
}
