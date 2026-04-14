package media

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/evey-project/evey/internal/domain/shared"
)

type ShareService struct {
	repo shared.ShareLinkRepository
}

func NewShareService(repo shared.ShareLinkRepository) *ShareService {
	return &ShareService{repo: repo}
}

func (s *ShareService) Create(ctx context.Context, mediaItemID, createdBy string, duration time.Duration, maxViews *int) (*shared.ShareLink, error) {
	if duration <= 0 || duration > 72*time.Hour {
		return nil, errors.New("share link duration must be between 0 and 72 hours")
	}
	link := &shared.ShareLink{
		ID:          uuid.New().String(),
		Token:       uuid.New().String(),
		MediaItemID: shared.MediaItemID(mediaItemID),
		CreatedBy:   shared.UserID(createdBy),
		ExpiresAt:   time.Now().Add(duration),
		MaxViews:    maxViews,
		ViewCount:   0,
		CreatedAt:   time.Now(),
	}
	if err := s.repo.Create(ctx, link); err != nil {
		return nil, err
	}
	return link, nil
}

func (s *ShareService) GetByID(ctx context.Context, id string) (*shared.ShareLink, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *ShareService) Validate(ctx context.Context, token string) (*shared.ShareLink, error) {
	link, err := s.repo.GetByToken(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("share link not found: %w", err)
	}
	if link.IsExpired() {
		return nil, errors.New("share link expired")
	}
	if link.IsViewLimitReached() {
		return nil, errors.New("share link view limit reached")
	}
	if err := s.repo.IncrementViewCount(ctx, token); err != nil {
		return nil, err
	}
	return link, nil
}

func (s *ShareService) List(ctx context.Context, userID string) ([]*shared.ShareLink, error) {
	return s.repo.ListByUser(ctx, userID)
}

func (s *ShareService) ListAll(ctx context.Context) ([]*shared.ShareLink, error) {
	return s.repo.ListAll(ctx)
}

func (s *ShareService) Revoke(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

func (s *ShareService) CleanupExpired(ctx context.Context) (int, error) {
	return s.repo.DeleteExpired(ctx)
}
