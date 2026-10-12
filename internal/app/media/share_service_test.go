package media_test

import (
	"context"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/app/media"
	"github.com/eveyproject/evey/internal/domain/shared"
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

func (r *fakeShareLinkRepo) ConsumeView(_ context.Context, token string) (*shared.ShareLink, error) {
	for _, l := range r.links {
		if l.Token == token {
			if l.IsExpired() || l.IsViewLimitReached() {
				return nil, shared.ErrNotFound
			}
			l.ViewCount++
			return l, nil
		}
	}
	return nil, shared.ErrNotFound
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
	assert.Equal(t, shared.MediaItemID("media-1"), link.MediaItemID)
	assert.Equal(t, shared.UserID("admin-1"), link.CreatedBy)

	validated, err := svc.Validate(ctx, link.Token)
	require.NoError(t, err)
	assert.Equal(t, shared.MediaItemID("media-1"), validated.MediaItemID)
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

// 0 and 100h were checked, the boundary itself never was - so mutation
// (2026-08-01) could move either end of the range with the suite still green.
// This is the cap on how long a PUBLIC link stays reachable, so drift here
// widens unauthenticated exposure without anything saying so.
func TestShareService_Create_DurationBoundary(t *testing.T) {
	repo := &fakeShareLinkRepo{}
	svc := media.NewShareService(repo)
	ctx := context.Background()

	link, err := svc.Create(ctx, "media-1", "admin-1", 72*time.Hour, nil)
	require.NoError(t, err, "exactly 72h is the documented maximum and must be accepted")
	require.NotNil(t, link)

	_, err = svc.Create(ctx, "media-1", "admin-1", 72*time.Hour+time.Second, nil)
	require.Error(t, err, "one second past the cap must be refused, or the ceiling is decorative")

	_, err = svc.Create(ctx, "media-1", "admin-1", -time.Hour, nil)
	require.Error(t, err, "a negative duration would mint an already-expired link")
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

// A refused download spends nothing: the counter stays at the limit instead
// of creeping past it with every rejected attempt.
func TestShareService_Validate_RefusalDoesNotSpendView(t *testing.T) {
	repo := &fakeShareLinkRepo{}
	svc := media.NewShareService(repo)
	ctx := context.Background()

	maxViews := 1
	link, err := svc.Create(ctx, "media-1", "admin-1", 24*time.Hour, &maxViews)
	require.NoError(t, err)

	_, err = svc.Validate(ctx, link.Token)
	require.NoError(t, err)
	for range 3 {
		_, err = svc.Validate(ctx, link.Token)
		require.ErrorIs(t, err, media.ErrShareViewLimit)
	}

	got, err := repo.GetByToken(ctx, link.Token)
	require.NoError(t, err)
	assert.Equal(t, 1, got.ViewCount)
}

// Deterministic timestamp via injected clock - proves the share's
// ExpiresAt = now()+duration, not "close to now+duration".
func TestShareService_Create_UsesInjectedClock(t *testing.T) {
	repo := &fakeShareLinkRepo{}
	fixed := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	svc := media.NewShareService(repo, media.WithClock(func() time.Time { return fixed }))

	link, err := svc.Create(context.Background(), "media-1", "admin-1", 24*time.Hour, nil)
	require.NoError(t, err)
	assert.Equal(t, fixed, link.CreatedAt)
	assert.Equal(t, fixed.Add(24*time.Hour), link.ExpiresAt)
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

func TestShareService_GetByID(t *testing.T) {
	repo := &fakeShareLinkRepo{}
	svc := media.NewShareService(repo)
	ctx := context.Background()

	link, err := svc.Create(ctx, "media-1", "admin-1", 24*time.Hour, nil)
	require.NoError(t, err)

	got, err := svc.GetByID(ctx, link.ID)
	require.NoError(t, err)
	assert.Equal(t, link.ID, got.ID)

	_, err = svc.GetByID(ctx, "not-exist")
	require.Error(t, err)
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

// ValidateShareLink (gRPC) is reachable by any authenticated user and is
// documented in the RBAC table as "harmless metadata". It shared Validate with
// the real download path, so a peek consumed a view: on a maxViews=1 link, any
// logged-in user could burn the single view before the intended recipient - who
// needs no account at all - ever opened the download URL.
func TestShareService_Peek_DoesNotConsumeAView(t *testing.T) {
	repo := &fakeShareLinkRepo{}
	svc := media.NewShareService(repo)
	ctx := context.Background()

	maxViews := 1
	link, _ := svc.Create(ctx, "media-1", "admin-1", 24*time.Hour, &maxViews)

	// Peeking repeatedly must not spend the allowance.
	for range 3 {
		peeked, err := svc.Peek(ctx, link.Token)
		require.NoError(t, err)
		assert.Equal(t, link.Token, peeked.Token)
	}

	// The real download still gets its one view.
	_, err := svc.Validate(ctx, link.Token)
	require.NoError(t, err, "the recipient's single view must survive any number of peeks")

	// ...and only one.
	_, err = svc.Validate(ctx, link.Token)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "view limit")
}

func TestShareService_Peek_StillRejectsExpiredAndExhausted(t *testing.T) {
	repo := &fakeShareLinkRepo{}
	svc := media.NewShareService(repo)
	ctx := context.Background()

	maxViews := 1
	link, _ := svc.Create(ctx, "media-1", "admin-1", 24*time.Hour, &maxViews)
	_, err := svc.Validate(ctx, link.Token)
	require.NoError(t, err)

	// A peek must not become a way to read an exhausted link either.
	_, err = svc.Peek(ctx, link.Token)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "view limit")
}
