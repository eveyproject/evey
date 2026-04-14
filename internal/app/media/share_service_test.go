package media_test

import (
	"context"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/app/media"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeShareLinkRepo struct {
	links []*shared.ShareLink
}

func (r *fakeShareLinkRepo) Create(_ context.Context, link *shared.ShareLink) error {
	r.links = append(r.links, link)
	return nil
}

func (r *fakeShareLinkRepo) GetByID(_ context.Context, id string) (*shared.ShareLink, error) {
	for _, l := range r.links {
		if l.ID == id {
			return l, nil
		}
	}
	return nil, shared.ErrNotFound
}

func (r *fakeShareLinkRepo) GetByToken(_ context.Context, token string) (*shared.ShareLink, error) {
	for _, l := range r.links {
		if l.Token == token {
			return l, nil
		}
	}
	return nil, shared.ErrNotFound
}

func (r *fakeShareLinkRepo) ListByUser(_ context.Context, userID string) ([]*shared.ShareLink, error) {
	var result []*shared.ShareLink
	for _, l := range r.links {
		if l.CreatedBy == shared.UserID(userID) {
			result = append(result, l)
		}
	}
	return result, nil
}

func (r *fakeShareLinkRepo) ListAll(_ context.Context) ([]*shared.ShareLink, error) {
	return r.links, nil
}

func (r *fakeShareLinkRepo) IncrementViewCount(_ context.Context, token string) error {
	for _, l := range r.links {
		if l.Token == token {
			l.ViewCount++
			return nil
		}
	}
	return nil
}

func (r *fakeShareLinkRepo) Delete(_ context.Context, id string) error {
	for i, l := range r.links {
		if l.ID == id {
			r.links = append(r.links[:i], r.links[i+1:]...)
			return nil
		}
	}
	return nil
}

func (r *fakeShareLinkRepo) DeleteExpired(_ context.Context) (int, error) {
	var kept []*shared.ShareLink
	deleted := 0
	for _, l := range r.links {
		if l.IsExpired() {
			deleted++
		} else {
			kept = append(kept, l)
		}
	}
	r.links = kept
	return deleted, nil
}

func TestShareService_CreateAndValidate(t *testing.T) {
	repo := &fakeShareLinkRepo{}
	svc := media.NewShareService(repo)
	ctx := context.Background()

	link, err := svc.Create(ctx, "media-1", "admin-1", 48*time.Hour, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, link.Token)
	assert.Equal(t, "media-1", link.MediaItemID)
	assert.Equal(t, "admin-1", link.CreatedBy)

	validated, err := svc.Validate(ctx, link.Token)
	require.NoError(t, err)
	assert.Equal(t, "media-1", validated.MediaItemID)
	assert.Equal(t, 1, validated.ViewCount)
}

func TestShareService_Create_InvalidDuration(t *testing.T) {
	repo := &fakeShareLinkRepo{}
	svc := media.NewShareService(repo)

	_, err := svc.Create(context.Background(), "media-1", "admin-1", 0, nil)
	require.Error(t, err)

	_, err = svc.Create(context.Background(), "media-1", "admin-1", 100*time.Hour, nil)
	require.Error(t, err)
}

func TestShareService_Validate_Expired(t *testing.T) {
	repo := &fakeShareLinkRepo{}
	svc := media.NewShareService(repo)
	ctx := context.Background()

	link, _ := svc.Create(ctx, "media-1", "admin-1", 1*time.Hour, nil)
	// Manually expire
	link.ExpiresAt = time.Now().Add(-1 * time.Minute)

	_, err := svc.Validate(ctx, link.Token)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

func TestShareService_Validate_ViewLimit(t *testing.T) {
	repo := &fakeShareLinkRepo{}
	svc := media.NewShareService(repo)
	ctx := context.Background()

	maxViews := 2
	link, _ := svc.Create(ctx, "media-1", "admin-1", 24*time.Hour, &maxViews)

	_, err := svc.Validate(ctx, link.Token)
	require.NoError(t, err)

	_, err = svc.Validate(ctx, link.Token)
	require.NoError(t, err)

	_, err = svc.Validate(ctx, link.Token)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "view limit")
}

func TestShareService_Revoke(t *testing.T) {
	repo := &fakeShareLinkRepo{}
	svc := media.NewShareService(repo)
	ctx := context.Background()

	link, _ := svc.Create(ctx, "media-1", "admin-1", 24*time.Hour, nil)

	err := svc.Revoke(ctx, link.ID)
	require.NoError(t, err)

	_, err = svc.Validate(ctx, link.Token)
	assert.Error(t, err)
}

func TestShareService_List(t *testing.T) {
	repo := &fakeShareLinkRepo{}
	svc := media.NewShareService(repo)
	ctx := context.Background()

	svc.Create(ctx, "media-1", "admin-1", 24*time.Hour, nil)
	svc.Create(ctx, "media-2", "admin-1", 24*time.Hour, nil)
	svc.Create(ctx, "media-3", "admin-2", 24*time.Hour, nil)

	links, err := svc.List(ctx, "admin-1")
	require.NoError(t, err)
	assert.Len(t, links, 2)

	all, err := svc.ListAll(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 3)
}

func TestShareService_CleanupExpired(t *testing.T) {
	repo := &fakeShareLinkRepo{}
	svc := media.NewShareService(repo)
	ctx := context.Background()

	link1, _ := svc.Create(ctx, "media-1", "admin-1", 24*time.Hour, nil)
	svc.Create(ctx, "media-2", "admin-1", 24*time.Hour, nil)

	// Manually expire link1
	link1.ExpiresAt = time.Now().Add(-1 * time.Minute)

	deleted, err := svc.CleanupExpired(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, deleted)

	all, _ := svc.ListAll(ctx)
	assert.Len(t, all, 1)
}
