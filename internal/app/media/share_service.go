package media

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
	"github.com/google/uuid"
)

// Sentinel errors exposed for handlers to map with errors.Is instead of
// string-matching on the message. Adding a new failure mode here stays
// a one-liner at the handler site.
var (
	ErrShareExpired       = errors.New("share link expired")
	ErrShareViewLimit     = errors.New("share link view limit reached")
	ErrShareDurationRange = errors.New("share link duration must be between 0 and 72 hours")
)

type ShareService struct {
	repo shared.ShareLinkRepository
	now  func() time.Time
}

type ShareOption func(*ShareService)

// WithClock overrides the time source; tests use it to get deterministic
// ExpiresAt / CreatedAt values.
func WithClock(now func() time.Time) ShareOption {
	return func(s *ShareService) { s.now = now }
}

func NewShareService(repo shared.ShareLinkRepository, opts ...ShareOption) *ShareService {
	s := &ShareService{repo: repo, now: time.Now}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *ShareService) Create(ctx context.Context, mediaItemID, createdBy string, duration time.Duration, maxViews *int) (*shared.ShareLink, error) {
	ctx, span := tracing.StartSpan(ctx, "app.share.Create")
	defer span.End()
	if duration <= 0 || duration > 72*time.Hour {
		return nil, ErrShareDurationRange
	}
	now := s.now()
	link := &shared.ShareLink{
		ID:          uuid.New().String(),
		Token:       uuid.New().String(),
		MediaItemID: shared.MediaItemID(mediaItemID),
		CreatedBy:   shared.UserID(createdBy),
		ExpiresAt:   now.Add(duration),
		MaxViews:    maxViews,
		ViewCount:   0,
		CreatedAt:   now,
	}
	if err := s.repo.Create(ctx, link); err != nil {
		return nil, err
	}
	return link, nil
}

func (s *ShareService) GetByID(ctx context.Context, id string) (*shared.ShareLink, error) {
	ctx, span := tracing.StartSpan(ctx, "app.share.GetByID")
	defer span.End()
	return s.repo.GetByID(ctx, id)
}

func (s *ShareService) Validate(ctx context.Context, token string) (*shared.ShareLink, error) {
	ctx, span := tracing.StartSpan(ctx, "app.share.Validate")
	defer span.End()
	link, err := s.repo.GetByToken(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("share link not found: %w", err)
	}
	if link.IsExpired() {
		return nil, ErrShareExpired
	}
	if link.IsViewLimitReached() {
		return nil, ErrShareViewLimit
	}
	if err := s.repo.IncrementViewCount(ctx, token); err != nil {
		return nil, err
	}
	// Re-check after the increment: two concurrent callers could both pass
	// the IsViewLimitReached() check above on the same pre-increment row,
	// then both increment and both get served - silently exceeding maxViews
	// by one. Refetch and reject the loser.
	updated, err := s.repo.GetByToken(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("share link not found after increment: %w", err)
	}
	if updated.IsExpired() {
		return nil, ErrShareExpired
	}
	if updated.MaxViews != nil && updated.ViewCount > *updated.MaxViews {
		return nil, ErrShareViewLimit
	}
	return updated, nil
}

func (s *ShareService) List(ctx context.Context, userID string) ([]*shared.ShareLink, error) {
	ctx, span := tracing.StartSpan(ctx, "app.share.List")
	defer span.End()
	return s.repo.ListByUser(ctx, userID)
}

func (s *ShareService) ListAll(ctx context.Context) ([]*shared.ShareLink, error) {
	ctx, span := tracing.StartSpan(ctx, "app.share.ListAll")
	defer span.End()
	return s.repo.ListAll(ctx)
}

func (s *ShareService) Revoke(ctx context.Context, id string) error {
	ctx, span := tracing.StartSpan(ctx, "app.share.Revoke")
	defer span.End()
	return s.repo.Delete(ctx, id)
}

func (s *ShareService) CleanupExpired(ctx context.Context) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "app.share.CleanupExpired")
	defer span.End()
	return s.repo.DeleteExpired(ctx)
}
