package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/activity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlayHistoryRepo_List_Basic(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	sessionRepo := postgres.NewPlaySessionRepository(db)
	historyRepo := postgres.NewPlayHistoryRepository(db)
	ctx := context.Background()

	u := newTestUser("phu1")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("php1", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("phlib1")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("phmi1", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	session := newTestPlaySession("phps1", string(p.ID), string(item.ID))
	require.NoError(t, sessionRepo.Create(ctx, session))

	entries, err := historyRepo.List(ctx, activity.PlayHistoryFilter{ProfileID: p.ID})
	require.NoError(t, err)
	require.Len(t, entries, 1)

	e := entries[0]
	assert.Equal(t, "phps1", e.ID)
	assert.Equal(t, p.ID, e.ProfileID)
	assert.Equal(t, string(item.ID), string(e.MediaItemID))
	assert.Equal(t, item.Title, e.MediaTitle)
	assert.Equal(t, string(item.Type), e.MediaType)
	assert.NotEmpty(t, e.PlayMethod)
}

func TestPlayHistoryRepo_List_FilterByMediaItemID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	sessionRepo := postgres.NewPlaySessionRepository(db)
	historyRepo := postgres.NewPlayHistoryRepository(db)
	ctx := context.Background()

	u := newTestUser("phu2")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("php2", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("phlib2")
	require.NoError(t, libRepo.Create(ctx, lib))
	itemA := newTestMediaItem("phmi2a", lib.ID)
	itemB := newTestMediaItem("phmi2b", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, itemA))
	require.NoError(t, itemRepo.Create(ctx, itemB))

	require.NoError(t, sessionRepo.Create(ctx, newTestPlaySession("phps2a", string(p.ID), string(itemA.ID))))
	require.NoError(t, sessionRepo.Create(ctx, newTestPlaySession("phps2b", string(p.ID), string(itemB.ID))))

	entries, err := historyRepo.List(ctx, activity.PlayHistoryFilter{
		ProfileID:   p.ID,
		MediaItemID: itemA.ID,
	})
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, itemA.ID, entries[0].MediaItemID)
}

func TestPlayHistoryRepo_List_Pagination(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	sessionRepo := postgres.NewPlaySessionRepository(db)
	historyRepo := postgres.NewPlayHistoryRepository(db)
	ctx := context.Background()

	u := newTestUser("phu3")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("php3", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("phlib3")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("phmi3", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	base := time.Now().UTC().Truncate(time.Millisecond)
	for i, id := range []string{"phps3a", "phps3b", "phps3c"} {
		s := newTestPlaySession(id, string(p.ID), string(item.ID))
		s.StartedAt = base.Add(time.Duration(i) * time.Minute)
		require.NoError(t, sessionRepo.Create(ctx, s))
	}

	page1, err := historyRepo.List(ctx, activity.PlayHistoryFilter{ProfileID: p.ID, Limit: 2})
	require.NoError(t, err)
	assert.Len(t, page1, 2)

	page2, err := historyRepo.List(ctx, activity.PlayHistoryFilter{ProfileID: p.ID, Limit: 2, Offset: 2})
	require.NoError(t, err)
	assert.Len(t, page2, 1)

	// no overlap between pages
	assert.NotEqual(t, page1[0].ID, page2[0].ID)
	assert.NotEqual(t, page1[1].ID, page2[0].ID)
}

func TestPlayHistoryRepo_List_Empty(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	historyRepo := postgres.NewPlayHistoryRepository(db)
	ctx := context.Background()

	u := newTestUser("phu4")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("php4", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))

	entries, err := historyRepo.List(ctx, activity.PlayHistoryFilter{ProfileID: p.ID})
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestPlayHistoryRepo_List_TimeRange(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	sessionRepo := postgres.NewPlaySessionRepository(db)
	historyRepo := postgres.NewPlayHistoryRepository(db)
	ctx := context.Background()

	u := newTestUser("phu5")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("php5", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("phlib5")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("phmi5", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	old := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Millisecond)
	recent := time.Now().UTC().Truncate(time.Millisecond)

	sOld := newTestPlaySession("phps5old", string(p.ID), string(item.ID))
	sOld.StartedAt = old
	require.NoError(t, sessionRepo.Create(ctx, sOld))

	sRecent := newTestPlaySession("phps5new", string(p.ID), string(item.ID))
	sRecent.StartedAt = recent
	require.NoError(t, sessionRepo.Create(ctx, sRecent))

	// only fetch the recent one using From filter
	cutoff := time.Now().UTC().Add(-1 * time.Hour)
	entries, err := historyRepo.List(ctx, activity.PlayHistoryFilter{
		ProfileID: p.ID,
		From:      &cutoff,
	})
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "phps5new", entries[0].ID)
}

func TestPlayHistoryRepo_List_DefaultLimit(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	sessionRepo := postgres.NewPlaySessionRepository(db)
	historyRepo := postgres.NewPlayHistoryRepository(db)
	ctx := context.Background()

	u := newTestUser("phu6")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("php6", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("phlib6")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("phmi6", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	require.NoError(t, sessionRepo.Create(ctx, newTestPlaySession("phps6a", string(p.ID), string(item.ID))))
	require.NoError(t, sessionRepo.Create(ctx, newTestPlaySession("phps6b", string(p.ID), string(item.ID))))

	// limit 0 should default to 500 — we just confirm it returns all
	entries, err := historyRepo.List(ctx, activity.PlayHistoryFilter{ProfileID: p.ID, Limit: 0})
	require.NoError(t, err)
	assert.Len(t, entries, 2)
}
