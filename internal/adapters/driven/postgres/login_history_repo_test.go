package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/gravity-zero/evey/internal/adapters/driven/postgres"
	"github.com/gravity-zero/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/gravity-zero/evey/internal/domain/activity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newLoginEntry(id, userID string, success bool, createdAt time.Time) *activity.LoginEntry {
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

func TestLoginHistoryRepo_CountFailedSince(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	repo := postgres.NewLoginHistoryRepository(db)
	ctx := context.Background()

	u := newTestUser("lh-u3")
	require.NoError(t, userRepo.Create(ctx, u))

	now := time.Now().UTC()
	before := now.Add(-10 * time.Minute)

	require.NoError(t, repo.Create(ctx, newLoginEntry("lh-f1", u.ID, false, now.Add(-5*time.Minute))))
	require.NoError(t, repo.Create(ctx, newLoginEntry("lh-f2", u.ID, false, now.Add(-3*time.Minute))))
	require.NoError(t, repo.Create(ctx, newLoginEntry("lh-f3", u.ID, true, now.Add(-1*time.Minute))))

	count, err := repo.CountFailedSince(ctx, u.ID, before)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	after := now.Add(time.Minute)
	count, err = repo.CountFailedSince(ctx, u.ID, after)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
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
