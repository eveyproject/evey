package favorite

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/eveyproject/evey/internal/domain/library/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestService() *Service {
	return NewService(fakes.NewFakeFavoriteRepository())
}

func TestToggle_AddAndRemove(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	added, err := svc.Toggle(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.True(t, added)

	ok, err := svc.IsFavorite(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.True(t, ok)

	removed, err := svc.Toggle(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.False(t, removed)

	ok, err = svc.IsFavorite(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestList(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	_, _ = svc.Toggle(ctx, "p1", "m1")
	_, _ = svc.Toggle(ctx, "p1", "m2")
	_, _ = svc.Toggle(ctx, "p1", "m3")
	_, _ = svc.Toggle(ctx, "p2", "m1")

	items, err := svc.List(ctx, "p1", 50, 0)
	require.NoError(t, err)
	assert.Len(t, items, 3)

	items, err = svc.List(ctx, "p2", 50, 0)
	require.NoError(t, err)
	assert.Len(t, items, 1)
}

func TestList_DefaultLimit(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	items, err := svc.List(ctx, "p1", 0, 0)
	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestIsFavorite_NotFound(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	ok, err := svc.IsFavorite(ctx, "p1", "m999")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestToggle_RepoError_OnIsFavorite(t *testing.T) {
	t.Parallel()
	repo := fakes.NewFakeFavoriteRepository()
	svc := NewService(repo)

	repo.Err = errors.New("db error")

	_, err := svc.Toggle(context.Background(), "p1", "m1")
	require.Error(t, err)
}

func TestList_LimitCappedAt500(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	ctx := context.Background()

	for i := range 10 {
		_, _ = svc.Toggle(ctx, "p1", string(rune('a'+i)))
	}

	// limit > 500 should be capped to 50 (the default cap in service)
	items, err := svc.List(ctx, "p1", 600, 0)
	require.NoError(t, err)
	// only 10 items total, all should be returned regardless of cap
	assert.Len(t, items, 10)
}

func TestList_LimitZeroUsesDefault(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	ctx := context.Background()

	for i := range 3 {
		_, _ = svc.Toggle(ctx, "p1", string(rune('a'+i)))
	}

	// limit=0 triggers the default (50)
	items, err := svc.List(ctx, "p1", 0, 0)
	require.NoError(t, err)
	assert.Len(t, items, 3)
}

func TestIsFavorite_DifferentProfiles(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	ctx := context.Background()

	_, _ = svc.Toggle(ctx, "p1", "m1")

	ok1, err := svc.IsFavorite(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.True(t, ok1)

	ok2, err := svc.IsFavorite(ctx, "p2", "m1")
	require.NoError(t, err)
	assert.False(t, ok2, "different profile should not see p1's favorites")
}

func TestToggle_MultipleItems_IndependentState(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	ctx := context.Background()

	_, _ = svc.Toggle(ctx, "p1", "m1")
	_, _ = svc.Toggle(ctx, "p1", "m2")

	// remove m1 but not m2
	_, _ = svc.Toggle(ctx, "p1", "m1")

	ok1, _ := svc.IsFavorite(ctx, "p1", "m1")
	ok2, _ := svc.IsFavorite(ctx, "p1", "m2")
	assert.False(t, ok1)
	assert.True(t, ok2)
}

func TestToggle_Idempotent_AddTwice(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	added1, err := svc.Toggle(ctx, "p1", "m1")
	if err != nil {
		t.Fatal(err)
	}
	if !added1 {
		t.Fatal("expected added=true on first toggle")
	}

	// toggle off then back on
	removed, err := svc.Toggle(ctx, "p1", "m1")
	if err != nil {
		t.Fatal(err)
	}
	if removed {
		t.Fatal("expected added=false when toggling off")
	}

	added2, err := svc.Toggle(ctx, "p1", "m1")
	if err != nil {
		t.Fatal(err)
	}
	if !added2 {
		t.Fatal("expected added=true on re-add")
	}

	ok, _ := svc.IsFavorite(ctx, "p1", "m1")
	if !ok {
		t.Fatal("expected favorite after re-add")
	}
}

func TestList_LimitClamped(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	// zero limit gets clamped to 50, should return all items up to 50
	for i := range 3 {
		svc.Toggle(ctx, "p1", "m"+string(rune('a'+i)))
	}

	items, err := svc.List(ctx, "p1", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items with limit=0 (clamped to 50), got %d", len(items))
	}
}

func TestList_LimitOverMax(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	for i := range 5 {
		svc.Toggle(ctx, "p1", "item"+string(rune('a'+i)))
	}

	// limit > 500 gets clamped to 50
	items, err := svc.List(ctx, "p1", 600, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 5 {
		t.Fatalf("expected 5 items, got %d", len(items))
	}
}

// TestSetFavorite_ConcurrentAddIsIdempotent proves the race behind "a retry
// after timeout inverts the favorite": two callers both wanting the item
// favorited (e.g. the original request and a client retry after a lost
// response) must never leave the item un-favorited, no matter how they
// interleave. Toggle's read-then-flip can't offer this guarantee even when
// callers guard it with an IsFavorite check first, because the check and the
// flip are two separate calls (TOCTOU). SetFavorite must go straight to the
// already-idempotent repo primitives (Add: ON CONFLICT DO NOTHING, Remove: a
// DELETE that no-ops when absent) so concurrent identical calls converge.
func TestSetFavorite_ConcurrentAddIsIdempotent(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	ctx := context.Background()

	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			_, err := svc.SetFavorite(ctx, "p1", "m1", true)
			assert.NoError(t, err)
		}()
	}
	wg.Wait()

	ok, err := svc.IsFavorite(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.True(t, ok, "concurrent duplicate 'add' calls must converge to favorited")
}

func TestSetFavorite_RepeatedRemoveStaysIdempotent(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	ctx := context.Background()

	_, err := svc.SetFavorite(ctx, "p1", "m1", true)
	require.NoError(t, err)

	for range 3 {
		_, removeErr := svc.SetFavorite(ctx, "p1", "m1", false)
		require.NoError(t, removeErr)
	}

	ok, err := svc.IsFavorite(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestSetFavorite_ReturnsTargetState(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	ctx := context.Background()

	added, err := svc.SetFavorite(ctx, "p1", "m1", true)
	require.NoError(t, err)
	assert.True(t, added)

	removed, err := svc.SetFavorite(ctx, "p1", "m1", false)
	require.NoError(t, err)
	assert.False(t, removed)
}

func TestList_Pagination(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	for _, id := range []string{"m1", "m2", "m3", "m4"} {
		svc.Toggle(ctx, "p1", id)
	}

	page1, err := svc.List(ctx, "p1", 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page1) != 2 {
		t.Fatalf("expected 2 items on page1, got %d", len(page1))
	}

	page2, err := svc.List(ctx, "p1", 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page2) != 2 {
		t.Fatalf("expected 2 items on page2, got %d", len(page2))
	}
}

func TestIsFavoriteMany(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	_, err := svc.SetFavorite(ctx, "p1", "m1", true)
	require.NoError(t, err)
	// Another profile's row must not answer for p1.
	_, err = svc.SetFavorite(ctx, "p2", "m2", true)
	require.NoError(t, err)

	got, err := svc.IsFavoriteMany(ctx, "p1", []string{"m1", "m2", "m3"})
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"m1": true}, got)
}

// An empty batch must not reach the repository at all - some drivers turn an
// empty array into SQL NULL, where `= ANY(NULL)` is never true.
func TestIsFavoriteMany_EmptyShortCircuits(t *testing.T) {
	svc := newTestService()

	got, err := svc.IsFavoriteMany(context.Background(), "p1", nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}
