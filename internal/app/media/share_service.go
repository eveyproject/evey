package media

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
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
	link, err := shared.NewShareLink(
		uuid.New().String(),
		uuid.New().String(),
		shared.MediaItemID(mediaItemID),
		shared.UserID(createdBy),
		now.Add(duration),
		maxViews,
		now,
	)
	if err != nil {
		return nil, err
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

// Peek resolves a share link without spending a view. It applies the same
// expiry and view-limit gates as Validate, so it never exposes a link that is
// already dead - it just doesn't consume the allowance.
//
// Validate is for the act of collecting the share (the download); Peek is for
// merely describing it. They used to be the same call, which meant any
// authenticated user hitting the ValidateShareLink RPC burned a view off a
// link addressed to someone else.
func (s *ShareService) Peek(ctx context.Context, token string) (*shared.ShareLink, error) {
	ctx, span := tracing.StartSpan(ctx, "app.share.Peek")
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
	return link, nil
}

func (s *ShareService) Validate(ctx context.Context, token string) (*shared.ShareLink, error) {
	ctx, span := tracing.StartSpan(ctx, "app.share.Validate")
	defer span.End()
	// The repo spends the view and enforces expiry + limit in one statement;
	// a read-check-increment here let two concurrent downloads both pass.
	link, err := s.repo.ConsumeView(ctx, token)
	if err == nil {
		return link, nil
	}
	if !errors.Is(err, shared.ErrNotFound) {
		return nil, err
	}
	// Nothing was spent: read the row only to name the refusal.
	link, err = s.repo.GetByToken(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("share link not found: %w", err)
	}
	if link.IsExpired() {
		return nil, ErrShareExpired
	}
	return nil, ErrShareViewLimit
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
