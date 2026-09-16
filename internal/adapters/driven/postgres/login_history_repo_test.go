package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/activity"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newLoginEntry(id string, userID shared.UserID, success bool, createdAt time.Time) *activity.LoginEntry {
	e := &activity.LoginEntry{
		ID:         id,
		UserID:     userID,
		ClientIP:   "127.0.0.1",
		UserAgent:  "test-agent",
		AuthMethod: "local",
		Success:    success,
		CreatedAt:  createdAt.UTC().Truncate(time.Millisecond),
	}
	if !success {
		e.FailureReason = stringPtr("bad credentials")
	}
	return e
}

func TestLoginHistoryRepo_Create_GetByUserID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	repo := postgres.NewLoginHistoryRepository(db)
	ctx := context.Background()

	u := newTestUser("lh-u1")
	require.NoError(t, userRepo.Create(ctx, u))

	now := time.Now().UTC()
	e1 := newLoginEntry("lh-e1", u.ID, true, now.Add(-2*time.Minute))
	e2 := newLoginEntry("lh-e2", u.ID, false, now.Add(-1*time.Minute))
	e3 := newLoginEntry("lh-e3", u.ID, true, now)

	require.NoError(t, repo.Create(ctx, e1))
	require.NoError(t, repo.Create(ctx, e2))
	require.NoError(t, repo.Create(ctx, e3))

	entries, err := repo.GetByUserID(ctx, u.ID, 10, 0)
	require.NoError(t, err)
	require.Len(t, entries, 3)

	// DESC order: e3, e2, e1
	assert.Equal(t, "lh-e3", entries[0].ID)
	assert.Equal(t, "lh-e2", entries[1].ID)
	assert.Equal(t, "lh-e1", entries[2].ID)
}

func TestLoginHistoryRepo_GetByUserID_Pagination(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	repo := postgres.NewLoginHistoryRepository(db)
	ctx := context.Background()

	u := newTestUser("lh-u2")
	require.NoError(t, userRepo.Create(ctx, u))

	now := time.Now().UTC()
	ids := []string{"lh-p1", "lh-p2", "lh-p3", "lh-p4", "lh-p5"}
	for i, id := range ids {
		e := newLoginEntry(id, u.ID, true, now.Add(time.Duration(i)*time.Minute))
		require.NoError(t, repo.Create(ctx, e))
	}

	// DESC order: p5, p4, p3, p2, p1
	page1, err := repo.GetByUserID(ctx, u.ID, 2, 0)
	require.NoError(t, err)
	require.Len(t, page1, 2)
	assert.Equal(t, "lh-p5", page1[0].ID)
	assert.Equal(t, "lh-p4", page1[1].ID)

	page2, err := repo.GetByUserID(ctx, u.ID, 2, 2)
	require.NoError(t, err)
	require.Len(t, page2, 2)
	assert.Equal(t, "lh-p3", page2[0].ID)
	assert.Equal(t, "lh-p2", page2[1].ID)
}

func TestLoginHistoryRepo_GetByUserID_Empty(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	repo := postgres.NewLoginHistoryRepository(db)
	ctx := context.Background()

	u := newTestUser("lh-u4")
	require.NoError(t, userRepo.Create(ctx, u))

	entries, err := repo.GetByUserID(ctx, u.ID, 10, 0)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// The keyset walk of a user's login history: newest first, a cursor resumes
// strictly after the last row even when two rows share a timestamp, and the
// success/window filters narrow the walk without breaking it.
func TestLoginHistoryRepo_ListByUserIDCursor(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	u := newTestUser("lh-cur")
	require.NoError(t, postgres.NewUserRepository(db).Create(ctx, u))
	repo := postgres.NewLoginHistoryRepository(db)

	now := time.Now().UTC().Truncate(time.Millisecond)
	// e2 and e3 share a timestamp: the id tiebreak must keep the walk total.
	entries := []*activity.LoginEntry{
		newLoginEntry("lhc-1", u.ID, true, now.Add(-3*time.Hour)),
		newLoginEntry("lhc-2", u.ID, false, now.Add(-time.Hour)),
		newLoginEntry("lhc-3", u.ID, true, now.Add(-time.Hour)),
		newLoginEntry("lhc-4", u.ID, true, now),
	}
	for _, e := range entries {
		require.NoError(t, repo.Create(ctx, e))
	}
	ids := func(es []*activity.LoginEntry) []string {
		out := make([]string, 0, len(es))
		for _, e := range es {
			out = append(out, e.ID)
		}
		return out
	}

	page, err := repo.ListByUserIDCursor(ctx, u.ID, false, nil, nil, nil, 2)
	require.NoError(t, err)
	assert.Equal(t, []string{"lhc-4", "lhc-3"}, ids(page))
	cursor := &activity.LoginHistoryCursor{StartedAt: page[1].CreatedAt, EntryID: page[1].ID}
	page, err = repo.ListByUserIDCursor(ctx, u.ID, false, nil, nil, cursor, 2)
	require.NoError(t, err)
	assert.Equal(t, []string{"lhc-2", "lhc-1"}, ids(page), "the tied row is neither repeated nor skipped")
	cursor = &activity.LoginHistoryCursor{StartedAt: page[1].CreatedAt, EntryID: page[1].ID}
	page, err = repo.ListByUserIDCursor(ctx, u.ID, false, nil, nil, cursor, 2)
	require.NoError(t, err)
	assert.Empty(t, page)

	page, err = repo.ListByUserIDCursor(ctx, u.ID, true, nil, nil, nil, 10)
	require.NoError(t, err)
	assert.Equal(t, []string{"lhc-4", "lhc-3", "lhc-1"}, ids(page), "successOnly drops the failure")

	from, to := now.Add(-2*time.Hour), now.Add(-30*time.Minute)
	page, err = repo.ListByUserIDCursor(ctx, u.ID, false, &from, &to, nil, 10)
	require.NoError(t, err)
	assert.Equal(t, []string{"lhc-3", "lhc-2"}, ids(page), "the window keeps the middle pair")

	page, err = repo.ListByUserIDCursor(ctx, "nobody", false, nil, nil, nil, 10)
	require.NoError(t, err)
	assert.Empty(t, page)
}
