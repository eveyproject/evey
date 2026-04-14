package postgres_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/gravity-zero/evey/internal/adapters/driven/postgres"
	"github.com/gravity-zero/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/gravity-zero/evey/internal/domain/syncplay"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedSyncplayDeps(t *testing.T, db *sql.DB, profileID, userID, libraryID, mediaID string) {
	t.Helper()
	ctx := context.Background()
	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	require.NoError(t, userRepo.Create(ctx, newTestUser(userID)))
	require.NoError(t, profileRepo.Create(ctx, newTestProfile(profileID, userID)))
	require.NoError(t, libRepo.Create(ctx, newTestLibrary(libraryID)))
	require.NoError(t, itemRepo.Create(ctx, newTestMediaItem(mediaID, shared.LibraryID(libraryID))))
}

func TestSyncPlayRoomRepo_Create_Get(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	seedSyncplayDeps(t, db, "host-u1", "sp-user1", "sp-lib1", "media-1")

	repo := postgres.NewSyncPlayRoomRepo(db)
	ctx := context.Background()

	room := &syncplay.Room{
		ID:        "sp1",
		Code:      "ABCDEF",
		Name:      "Movie Night",
		HostID:    "host-u1",
		MediaID:   "media-1",
		Members:   []syncplay.Member{{UserID: "host-u1", Username: "host", JoinedAt: time.Now().UTC().Truncate(time.Millisecond)}},
		State:     syncplay.PlaybackState{Playing: false, PositionMs: 0},
		CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, repo.Create(ctx, room))

	got, err := repo.Get(ctx, "sp1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Movie Night", got.Name)
	assert.Len(t, got.Members, 1)
}

func TestSyncPlayRoomRepo_GetByCode(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	seedSyncplayDeps(t, db, "host-u2", "sp-user2", "sp-lib2", "media-2")

	repo := postgres.NewSyncPlayRoomRepo(db)
	ctx := context.Background()

	room := &syncplay.Room{
		ID:        "sp2",
		Code:      "XYZ789",
		Name:      "Watch Party",
		HostID:    "host-u2",
		MediaID:   "media-2",
		Members:   []syncplay.Member{},
		State:     syncplay.PlaybackState{},
		CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, repo.Create(ctx, room))

	got, err := repo.GetByCode(ctx, "XYZ789")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "sp2", got.ID)
}

func TestSyncPlayRoomRepo_AddMember_RemoveMember(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	seedSyncplayDeps(t, db, "host-u3", "sp-user3", "sp-lib3", "media-3")

	repo := postgres.NewSyncPlayRoomRepo(db)
	ctx := context.Background()

	room := &syncplay.Room{
		ID:        "sp3",
		Code:      "MEM123",
		Name:      "Room",
		HostID:    "host-u3",
		MediaID:   "media-3",
		Members:   []syncplay.Member{},
		State:     syncplay.PlaybackState{},
		CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, repo.Create(ctx, room))

	member := syncplay.Member{UserID: "guest-1", Username: "guest", JoinedAt: time.Now().UTC().Truncate(time.Millisecond)}
	require.NoError(t, repo.AddMember(ctx, "sp3", member))

	got, err := repo.Get(ctx, "sp3")
	require.NoError(t, err)
	assert.Len(t, got.Members, 1)

	require.NoError(t, repo.RemoveMember(ctx, "sp3", "guest-1"))
	got, err = repo.Get(ctx, "sp3")
	require.NoError(t, err)
	assert.Empty(t, got.Members)
}

func TestSyncPlayRoomRepo_UpdateState(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	seedSyncplayDeps(t, db, "host-u4", "sp-user4", "sp-lib4", "media-4")

	repo := postgres.NewSyncPlayRoomRepo(db)
	ctx := context.Background()

	room := &syncplay.Room{
		ID:        "sp4",
		Code:      "STA456",
		Name:      "StateRoom",
		HostID:    "host-u4",
		MediaID:   "media-4",
		Members:   []syncplay.Member{},
		State:     syncplay.PlaybackState{Playing: false, PositionMs: 0},
		CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, repo.Create(ctx, room))

	newState := syncplay.PlaybackState{Playing: true, PositionMs: 5000, UpdatedBy: "host-u4"}
	require.NoError(t, repo.UpdateState(ctx, "sp4", newState))

	got, err := repo.Get(ctx, "sp4")
	require.NoError(t, err)
	assert.True(t, got.State.Playing)
	assert.Equal(t, int64(5000), got.State.PositionMs)
}

func TestSyncPlayRoomRepo_Delete(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	seedSyncplayDeps(t, db, "host-u5", "sp-user5", "sp-lib5", "media-5")

	repo := postgres.NewSyncPlayRoomRepo(db)
	ctx := context.Background()

	room := &syncplay.Room{
		ID:        "sp5",
		Code:      "DEL000",
		Name:      "Temp",
		HostID:    "host-u5",
		MediaID:   "media-5",
		Members:   []syncplay.Member{},
		State:     syncplay.PlaybackState{},
		CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, repo.Create(ctx, room))
	require.NoError(t, repo.Delete(ctx, "sp5"))

	got, err := repo.Get(ctx, "sp5")
	require.NoError(t, err)
	assert.Nil(t, got)
}
