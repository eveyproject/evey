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

// hostUserID is an ACCOUNT: a room's host, like its members, is a user id
// (fk_syncplay_rooms_host_id references users since migration 000167).
func seedSyncplayDeps(t *testing.T, db *sql.DB, hostUserID, profileID, libraryID, mediaID string) {
	t.Helper()
	ctx := context.Background()
	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	require.NoError(t, userRepo.Create(ctx, newTestUser(hostUserID)))
	require.NoError(t, profileRepo.Create(ctx, newTestProfile(profileID, shared.UserID(hostUserID))))
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
	deleted, _, err := repo.RemoveMemberAndDeleteIfEmpty(ctx, "sp9", "guest-a")
	require.NoError(t, err)
	assert.False(t, deleted, "room with a remaining member must not be deleted")
	got, err := repo.Get(ctx, "sp9")
	require.NoError(t, err)
	assert.Len(t, got.Members, 1)

	// Removing the last member deletes the room in the same operation.
	deleted, _, err = repo.RemoveMemberAndDeleteIfEmpty(ctx, "sp9", "guest-b")
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

// TestSyncPlayRoomRepo_RemoveMemberAndDeleteIfEmpty_HostElection covers the
// three host-election cases inside the atomic FOR UPDATE transaction.
func TestSyncPlayRoomRepo_RemoveMemberAndDeleteIfEmpty_HostElection(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	seedSyncplayDeps(t, db, "he-host", "he-u1", "he-lib", "he-media")
	// Seed two additional accounts to be regular members (no library/media
	// needed): an elected host is written to host_id, which names an account.
	ctx := context.Background()
	require.NoError(t, postgres.NewUserRepository(db).Create(ctx, newTestUser("he-m2")))
	require.NoError(t, postgres.NewUserRepository(db).Create(ctx, newTestUser("he-m3")))

	repo := postgres.NewSyncPlayRoomRepo(db)
	now := time.Now().UTC().Truncate(time.Millisecond)

	// --- (a) host departs with ≥2 remaining members → host_id = oldest ---
	roomA := &syncplay.Room{
		ID:      "he-a",
		Code:    "HEA001",
		Name:    "Election A",
		HostID:  "he-host",
		MediaID: "he-media",
		Members: []syncplay.Member{
			// he-m2 joined earlier (smallest JoinedAt) → must be elected
			{UserID: "he-host", Username: "host", JoinedAt: now},
			{UserID: "he-m2", Username: "m2", JoinedAt: now.Add(-time.Second)},
			{UserID: "he-m3", Username: "m3", JoinedAt: now.Add(time.Second)},
		},
		State:     syncplay.PlaybackState{},
		CreatedAt: now,
	}
	require.NoError(t, repo.Create(ctx, roomA))

	deleted, newHostID, err := repo.RemoveMemberAndDeleteIfEmpty(ctx, "he-a", "he-host")
	require.NoError(t, err)
	assert.False(t, deleted, "room must not be deleted while members remain")
	assert.Equal(t, "he-m2", newHostID, "oldest remaining member must be elected host")

	gotA, err := repo.Get(ctx, "he-a")
	require.NoError(t, err)
	assert.Equal(t, shared.UserID("he-m2"), gotA.HostID, "host_id must be persisted atomically")
	assert.Len(t, gotA.Members, 2, "departing host must be removed from members")

	// --- (b) non-host departs → host_id unchanged, newHostID="" ---
	roomB := &syncplay.Room{
		ID:      "he-b",
		Code:    "HEB002",
		Name:    "Election B",
		HostID:  "he-host",
		MediaID: "he-media",
		Members: []syncplay.Member{
			{UserID: "he-host", Username: "host", JoinedAt: now},
			{UserID: "he-m2", Username: "m2", JoinedAt: now},
		},
		State:     syncplay.PlaybackState{},
		CreatedAt: now,
	}
	require.NoError(t, repo.Create(ctx, roomB))

	deleted, newHostID, err = repo.RemoveMemberAndDeleteIfEmpty(ctx, "he-b", "he-m2")
	require.NoError(t, err)
	assert.False(t, deleted)
	assert.Empty(t, newHostID, "no election when a non-host departs")

	gotB, err := repo.Get(ctx, "he-b")
	require.NoError(t, err)
	assert.Equal(t, shared.UserID("he-host"), gotB.HostID, "host_id must remain unchanged")

	// --- (c) last member departs → room deleted, newHostID="" ---
	roomC := &syncplay.Room{
		ID:      "he-c",
		Code:    "HEC003",
		Name:    "Election C",
		HostID:  "he-host",
		MediaID: "he-media",
		Members: []syncplay.Member{
			{UserID: "he-host", Username: "host", JoinedAt: now},
		},
		State:     syncplay.PlaybackState{},
		CreatedAt: now,
	}
	require.NoError(t, repo.Create(ctx, roomC))

	deleted, newHostID, err = repo.RemoveMemberAndDeleteIfEmpty(ctx, "he-c", "he-host")
	require.NoError(t, err)
	assert.True(t, deleted, "room must be deleted when last member departs")
	assert.Empty(t, newHostID)
	_, err = repo.Get(ctx, "he-c")
	require.Error(t, err, "room row must be gone after last member removal")
}

// Regression: an audio room's media_id references a music track, which is not
// a media_item. This used to violate fk_syncplay_rooms_media_id; the FK was
// dropped (migration 000100) since existence is validated in the handler per
// media_type. Seed only the host (no media_item) and assert the insert works.
func TestSyncPlayRoomRepo_Create_AudioMediaNotInMediaItems(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()

	require.NoError(t, postgres.NewUserRepository(db).Create(ctx, newTestUser("sp-audio-host")))

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

func TestSyncPlayRoomRepo_DeleteStaleEmptyRooms(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	seedSyncplayDeps(t, db, "host-u1", "sp-user1", "sp-lib1", "media-1")

	repo := postgres.NewSyncPlayRoomRepo(db)
	ctx := context.Background()

	old := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Millisecond)
	fresh := time.Now().UTC().Truncate(time.Millisecond)

	// stale + empty (nil members -> stored as JSON 'null'): swept.
	require.NoError(t, repo.Create(ctx, &syncplay.Room{
		ID: "stale-empty", Code: "STALE1", Name: "x", HostID: "host-u1", MediaID: "media-1",
		Members: nil, CreatedAt: old,
	}))
	// stale but with a live member: kept.
	require.NoError(t, repo.Create(ctx, &syncplay.Room{
		ID: "stale-full", Code: "STALE2", Name: "x", HostID: "host-u1", MediaID: "media-1",
		Members: []syncplay.Member{{UserID: "host-u1", Username: "h", JoinedAt: old}}, CreatedAt: old,
	}))
	// fresh + empty (inside the create->join window): kept.
	require.NoError(t, repo.Create(ctx, &syncplay.Room{
		ID: "fresh-empty", Code: "FRESH1", Name: "x", HostID: "host-u1", MediaID: "media-1",
		Members: nil, CreatedAt: fresh,
	}))

	ids, err := repo.DeleteStaleEmptyRooms(ctx, time.Now().Add(-time.Hour))
	require.NoError(t, err)
	// The IDS, not a count: each one may have minted guest stream tokens that
	// have to die with their room, and the janitor is the only path that ever
	// deletes a room nobody joined.
	assert.Equal(t, []string{"stale-empty"}, ids)

	_, err = repo.Get(ctx, "stale-empty")
	require.Error(t, err, "stale empty room should be deleted")
	_, err = repo.Get(ctx, "stale-full")
	require.NoError(t, err, "stale room with a member must be kept")
	_, err = repo.Get(ctx, "fresh-empty")
	require.NoError(t, err, "fresh empty room must be kept")
}

func TestSyncPlayRoomRepo_CountActiveByHost(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	seedSyncplayDeps(t, db, "cah-host", "cah-user", "cah-lib", "cah-media")
	repo := postgres.NewSyncPlayRoomRepo(db)
	ctx := context.Background()

	mk := func(id string) *syncplay.Room {
		return &syncplay.Room{
			ID: id, Code: id[:min(5, len(id))], Name: id, HostID: "cah-host", MediaID: "cah-media",
			State: syncplay.PlaybackState{}, CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
		}
	}
	require.NoError(t, repo.Create(ctx, mk("cah1")))
	require.NoError(t, repo.Create(ctx, mk("cah2")))

	n, err := repo.CountActiveByHost(ctx, "cah-host")
	require.NoError(t, err)
	assert.Equal(t, 2, n, "both rooms this host owns must be counted")

	n0, err := repo.CountActiveByHost(ctx, "someone-else")
	require.NoError(t, err)
	assert.Equal(t, 0, n0, "a host with no rooms counts zero")
}
