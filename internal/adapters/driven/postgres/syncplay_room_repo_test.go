package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/syncplay"
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
	require.NoError(t, profileRepo.Create(ctx, newTestProfile(profileID, shared.UserID(userID))))
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

func TestSyncPlayRoomRepo_RemoveMemberAndDeleteIfEmpty(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	seedSyncplayDeps(t, db, "host-u9", "sp-user9", "sp-lib9", "media-9")

	repo := postgres.NewSyncPlayRoomRepo(db)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Millisecond)
	room := &syncplay.Room{
		ID:      "sp9",
		Code:    "DIE999",
		Name:    "Room",
		HostID:  "host-u9",
		MediaID: "media-9",
		Members: []syncplay.Member{
			{UserID: "guest-a", Username: "a", JoinedAt: now},
			{UserID: "guest-b", Username: "b", JoinedAt: now},
		},
		State:     syncplay.PlaybackState{},
		CreatedAt: now,
	}
	require.NoError(t, repo.Create(ctx, room))

	// Removing one of two members keeps the room alive.
	deleted, err := repo.RemoveMemberAndDeleteIfEmpty(ctx, "sp9", "guest-a")
	require.NoError(t, err)
	assert.False(t, deleted, "room with a remaining member must not be deleted")
	got, err := repo.Get(ctx, "sp9")
	require.NoError(t, err)
	assert.Len(t, got.Members, 1)

	// Removing the last member deletes the room in the same operation.
	deleted, err = repo.RemoveMemberAndDeleteIfEmpty(ctx, "sp9", "guest-b")
	require.NoError(t, err)
	assert.True(t, deleted, "emptied room must be deleted, not left behind")
	_, err = repo.Get(ctx, "sp9")
	require.Error(t, err, "room should be gone after its last member was removed")
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

func TestSyncPlayRoomRepo_QueueAndTrackIndex(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	seedSyncplayDeps(t, db, "host-uq", "sp-userq", "sp-libq", "media-q1")
	itemRepo := postgres.NewMediaItemRepository(db)
	require.NoError(t, itemRepo.Create(context.Background(), newTestMediaItem("media-q2", shared.LibraryID("sp-libq"))))
	require.NoError(t, itemRepo.Create(context.Background(), newTestMediaItem("media-q3", shared.LibraryID("sp-libq"))))

	repo := postgres.NewSyncPlayRoomRepo(db)
	ctx := context.Background()

	plID := "pl-test-1"
	room := &syncplay.Room{
		ID:         "spq",
		Code:       "QUEUE1",
		Name:       "Queue Room",
		HostID:     "host-uq",
		MediaID:    "media-q1",
		MediaType:  "music",
		PlaylistID: &plID,
		Queue:      []shared.MediaItemID{"media-q1", "media-q2", "media-q3"},
		TrackIndex: 0,
		Members:    []syncplay.Member{},
		State:      syncplay.PlaybackState{},
		CreatedAt:  time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, repo.Create(ctx, room))

	got, err := repo.Get(ctx, "spq")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, []shared.MediaItemID{"media-q1", "media-q2", "media-q3"}, got.Queue)
	assert.Equal(t, 0, got.TrackIndex)
	require.NotNil(t, got.PlaylistID)
	assert.Equal(t, plID, *got.PlaylistID)

	// Advance to next track
	require.NoError(t, repo.UpdateTrackIndex(ctx, "spq", 1, "media-q2"))
	got, err = repo.Get(ctx, "spq")
	require.NoError(t, err)
	assert.Equal(t, 1, got.TrackIndex)
	assert.Equal(t, shared.MediaItemID("media-q2"), got.MediaID)
}

func TestSyncPlayRoomRepo_AdvanceTrackIndex_ConcurrentNoLostIncrement(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	seedSyncplayDeps(t, db, "host-ad", "sp-userad", "sp-libad", "media-a1")
	itemRepo := postgres.NewMediaItemRepository(db)
	require.NoError(t, itemRepo.Create(context.Background(), newTestMediaItem("media-a2", shared.LibraryID("sp-libad"))))
	require.NoError(t, itemRepo.Create(context.Background(), newTestMediaItem("media-a3", shared.LibraryID("sp-libad"))))

	repo := postgres.NewSyncPlayRoomRepo(db)
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, &syncplay.Room{
		ID: "spa", Code: "ADV1", Name: "Advance", HostID: "host-ad", MediaID: "media-a1", MediaType: "music",
		Queue: []shared.MediaItemID{"media-a1", "media-a2", "media-a3"}, TrackIndex: 0,
		Members: []syncplay.Member{}, State: syncplay.PlaybackState{}, CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
	}))

	// Two concurrent "next" advances must land on index 2, never 1: the old
	// read-modify-write in the handler let both read index 0 and write 1, losing
	// an increment. The atomic UPDATE serialises them on the row lock.
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			_, _, _, err := repo.AdvanceTrackIndex(ctx, "spa", 1)
			assert.NoError(t, err)
		})
	}
	wg.Wait()

	got, err := repo.Get(ctx, "spa")
	require.NoError(t, err)
	assert.Equal(t, 2, got.TrackIndex, "two concurrent advances must not lose an increment")
	assert.Equal(t, shared.MediaItemID("media-a3"), got.MediaID)

	// Advancing past the last track is a no-op (changed=false, index unchanged).
	_, _, changed, err := repo.AdvanceTrackIndex(ctx, "spa", 1)
	require.NoError(t, err)
	assert.False(t, changed, "advancing past the last track must be a no-op")
	got, err = repo.Get(ctx, "spa")
	require.NoError(t, err)
	assert.Equal(t, 2, got.TrackIndex)
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
	require.ErrorIs(t, err, shared.ErrNotFound)
	assert.Nil(t, got)
}

func TestSyncPlayRoomRepo_ConcurrentAddMember_NoLostUpdate(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	seedSyncplayDeps(t, db, "host-uc", "sp-user-c", "sp-libc", "media-c")

	repo := postgres.NewSyncPlayRoomRepo(db)
	ctx := context.Background()

	room := &syncplay.Room{
		ID:        "sp-conc",
		Code:      "CONC01",
		Name:      "ConcRoom",
		HostID:    "host-uc",
		MediaID:   "media-c",
		Members:   []syncplay.Member{},
		State:     syncplay.PlaybackState{},
		CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, repo.Create(ctx, room))

	const n = 8
	errs := make([]error, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		go func() {
			defer wg.Done()
			m := syncplay.Member{
				UserID:   shared.UserID(fmt.Sprintf("guest-%d", i)),
				Username: fmt.Sprintf("guest%d", i),
				JoinedAt: time.Now().UTC().Truncate(time.Millisecond),
			}
			errs[i] = repo.AddMember(ctx, "sp-conc", m)
		}()
	}
	wg.Wait()

	for i, err := range errs {
		require.NoError(t, err, "goroutine %d failed", i)
	}

	got, err := repo.Get(ctx, "sp-conc")
	require.NoError(t, err)
	// Without the FOR UPDATE fix, concurrent RMW would lose updates;
	// with it, all 8 distinct members must be present.
	assert.Len(t, got.Members, n)
}

// Regression: an audio room's media_id references a music track, which is not
// a media_item. This used to violate fk_syncplay_rooms_media_id; the FK was
// dropped (migration 000100) since existence is validated in the handler per
// media_type. Seed only the host (no media_item) and assert the insert works.
func TestSyncPlayRoomRepo_Create_AudioMediaNotInMediaItems(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()

	require.NoError(t, postgres.NewUserRepository(db).Create(ctx, newTestUser("sp-audio-user")))
	require.NoError(t, postgres.NewProfileRepository(db).Create(ctx, newTestProfile("sp-audio-host", shared.UserID("sp-audio-user"))))

	repo := postgres.NewSyncPlayRoomRepo(db)
	room := &syncplay.Room{
		ID:        "sp-audio",
		Code:      "MUSIC1",
		Name:      "Listen Together",
		HostID:    "sp-audio-host",
		MediaID:   "track-not-a-media-item",
		MediaType: "music",
		Members:   []syncplay.Member{},
		State:     syncplay.PlaybackState{},
		CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, repo.Create(ctx, room), "audio room with a non-media_item media_id must insert (FK dropped)")

	got, err := repo.Get(ctx, "sp-audio")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, shared.MediaItemID("track-not-a-media-item"), got.MediaID)
}
