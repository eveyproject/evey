package media_test

import (
	"sync"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/app/media"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStreamTokenStore_ConcurrentValidate_NoRace hammers Validate for the same
// token from many goroutines (the per-segment hot path) so `go test -race`
// catches any in-place mutation of the shared *StreamToken's ExpiresAt.
func TestStreamTokenStore_ConcurrentValidate_NoRace(t *testing.T) {
	store := media.NewStreamTokenStore(5 * time.Minute)
	defer store.Stop()

	token := store.Create("user-1", "profile-1", "media-1")

	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			for range 50 {
				if st, ok := store.Validate(token, "media-1"); ok {
					_ = st.ExpiresAt // read the field the renewal updates
				}
			}
		})
	}
	wg.Wait()

	_, ok := store.Validate(token, "media-1")
	assert.True(t, ok, "token still valid after concurrent renewals")
}

func TestStreamTokenStore_CreateAndValidate(t *testing.T) {
	store := media.NewStreamTokenStore(5 * time.Minute)
	defer store.Stop()

	token := store.Create("user-1", "profile-1", "media-1")
	require.NotEmpty(t, token)

	st, ok := store.Validate(token, "media-1")
	require.True(t, ok)
	assert.Equal(t, shared.UserID("user-1"), st.UserID)
	assert.Equal(t, shared.MediaItemID("media-1"), st.MediaItemID)
	assert.Equal(t, "stream", st.Scope)
}

func TestStreamTokenStore_ValidateWrongMedia(t *testing.T) {
	store := media.NewStreamTokenStore(5 * time.Minute)
	defer store.Stop()

	token := store.Create("user-1", "profile-1", "media-1")

	_, ok := store.Validate(token, "media-2")
	assert.False(t, ok, "wrong mediaItemID should fail")
}

func TestStreamTokenStore_CreateGuestToken(t *testing.T) {
	store := media.NewStreamTokenStore(5 * time.Minute)
	defer store.Stop()

	token := store.CreateGuestToken("media-1", "room-1", 3*time.Hour)
	require.NotEmpty(t, token)

	st, ok := store.Validate(token, "media-1")
	require.True(t, ok, "guest token must stream its own media")
	assert.Equal(t, shared.ScopeSyncplayGuest, st.Scope)
	assert.Equal(t, shared.MediaItemID("media-1"), st.MediaItemID)
	assert.Equal(t, "room-1", st.RoomID, "a guest token must name the room that minted it")
	assert.Empty(t, st.UserID, "guest token carries no user")
	assert.Empty(t, st.ProfileID, "guest token carries no profile")

	_, ok = store.Validate(token, "media-2")
	assert.False(t, ok, "guest token is scoped to its single media item")
}

// Closing the room is the host's only way to take back what a shared code gave
// out. A guest token carries no user, so RevokeByUser can never reach it - it
// returns 0 on an empty id by design - and before this the token simply
// outlived the room for its whole TTL.
func TestStreamTokenStore_RevokeByRoom(t *testing.T) {
	store := media.NewStreamTokenStore(5 * time.Minute)
	defer store.Stop()

	guestA := store.CreateGuestToken("media-1", "room-1", 3*time.Hour)
	guestB := store.CreateGuestToken("media-1", "room-1", 3*time.Hour)
	otherRoom := store.CreateGuestToken("media-2", "room-2", 3*time.Hour)
	member := store.Create("user-1", "profile-1", "media-1")

	assert.Equal(t, 0, store.RevokeByUser(""), "an empty user id must never sweep guest tokens")

	assert.Equal(t, 2, store.RevokeByRoom("room-1"))

	_, ok := store.Validate(guestA, "media-1")
	assert.False(t, ok, "guest token outlived its room")
	_, ok = store.Validate(guestB, "media-1")
	assert.False(t, ok, "guest token outlived its room")

	_, ok = store.Validate(otherRoom, "media-2")
	assert.True(t, ok, "another room's guests must be untouched")
	_, ok = store.Validate(member, "media-1")
	assert.True(t, ok, "a signed-in member's own token is not a guest token")

	assert.Equal(t, 0, store.RevokeByRoom(""), "an empty room id must sweep nothing")
}

func TestStreamTokenStore_ValidateAny(t *testing.T) {
	store := media.NewStreamTokenStore(5 * time.Minute)
	defer store.Stop()

	token := store.Create("user-1", "profile-1", "media-1")

	st, ok := store.ValidateAny(token)
	require.True(t, ok)
	assert.Equal(t, shared.MediaItemID("media-1"), st.MediaItemID)
}

// TestStreamTokenStore_Expired proves expiry deterministically by advancing a
// fake clock past the TTL - no time.Sleep, no wall-clock flakiness.
func TestStreamTokenStore_Expired(t *testing.T) {
	fakeNow := time.Now()
	store := media.NewStreamTokenStoreForTest(10*time.Millisecond, func() time.Time { return fakeNow })
	defer store.Stop()

	token := store.Create("user-1", "profile-1", "media-1")

	fakeNow = fakeNow.Add(20 * time.Millisecond)

	_, ok := store.Validate(token, "media-1")
	assert.False(t, ok, "expired token should fail")
}

// TestStreamTokenStore_RenewsTTL proves the sliding TTL deterministically: a
// Validate call before expiry must push ExpiresAt forward by another full TTL,
// so a subsequent instant past the original expiry is still valid.
func TestStreamTokenStore_RenewsTTL(t *testing.T) {
	fakeNow := time.Now()
	store := media.NewStreamTokenStoreForTest(50*time.Millisecond, func() time.Time { return fakeNow })
	defer store.Stop()

	token := store.Create("user-1", "profile-1", "media-1")

	// Access at 30ms - should renew to 80ms
	fakeNow = fakeNow.Add(30 * time.Millisecond)
	_, ok := store.Validate(token, "media-1")
	require.True(t, ok)

	// At 60ms (past original 50ms) - should still be valid due to renewal
	fakeNow = fakeNow.Add(30 * time.Millisecond)
	_, ok = store.Validate(token, "media-1")
	assert.True(t, ok, "token should be renewed after access")
}

// TestStreamTokenStore_RenewsBySlideTTLNotStoreDefault proves the sliding
// window uses the token's own TTL (e.g. the 12h cast window), not the store's
// default TTL - a regression here would silently downgrade cast tokens to the
// short default on their first segment fetch.
func TestStreamTokenStore_RenewsBySlideTTLNotStoreDefault(t *testing.T) {
	fakeNow := time.Now()
	store := media.NewStreamTokenStoreForTest(5*time.Minute, func() time.Time { return fakeNow })
	defer store.Stop()

	token := store.CreateWithTTL("user-1", "profile-1", "media-1", media.CastStreamTokenTTL)

	// Advance past the store default (5m) but well within the 12h cast TTL,
	// then renew.
	fakeNow = fakeNow.Add(10 * time.Minute)
	st, ok := store.Validate(token, "media-1")
	require.True(t, ok, "cast token must survive past the store default TTL")

	// The renewed ExpiresAt must be fakeNow + 12h, not fakeNow + 5m.
	assert.Equal(t, fakeNow.Add(media.CastStreamTokenTTL), st.ExpiresAt)

	// Advance another 11h (past 5m*2 but within another 12h window) - still valid.
	fakeNow = fakeNow.Add(11 * time.Hour)
	_, ok = store.Validate(token, "media-1")
	assert.True(t, ok, "cast token slides by its own 12h TTL, not the 5m store default")
}

// TestStreamTokenStore_PurgeExpired proves the cleanup logic directly at a
// controlled instant, without waiting on the cleanupLoop ticker.
func TestStreamTokenStore_PurgeExpired(t *testing.T) {
	fakeNow := time.Now()
	store := media.NewStreamTokenStoreForTest(10*time.Minute, func() time.Time { return fakeNow })
	defer store.Stop()

	expiring := store.Create("user-1", "profile-1", "media-1")
	longLived := store.CreateWithTTL("user-1", "profile-1", "media-2", time.Hour)

	fakeNow = fakeNow.Add(20 * time.Minute)
	store.PurgeExpiredForTest(fakeNow)

	if _, ok := store.ValidateAny(expiring); ok {
		t.Error("token past its TTL must be purged")
	}
	if _, ok := store.ValidateAny(longLived); !ok {
		t.Error("token still within its TTL must survive the purge")
	}
}

func TestStreamTokenStore_Revoke(t *testing.T) {
	store := media.NewStreamTokenStore(5 * time.Minute)
	defer store.Stop()

	token := store.Create("user-1", "profile-1", "media-1")

	store.Revoke(token)

	_, ok := store.Validate(token, "media-1")
	assert.False(t, ok, "revoked token should fail")
}

func TestStreamTokenStore_InvalidToken(t *testing.T) {
	store := media.NewStreamTokenStore(5 * time.Minute)
	defer store.Stop()

	_, ok := store.Validate("nonexistent-token", "media-1")
	assert.False(t, ok)
}

func TestStreamTokenStore_UniqueTokens(t *testing.T) {
	store := media.NewStreamTokenStore(5 * time.Minute)
	defer store.Stop()

	t1 := store.Create("user-1", "profile-1", "media-1")
	t2 := store.Create("user-1", "profile-1", "media-1")
	assert.NotEqual(t, t1, t2, "each call should generate a unique token")
}

// Validate(token, "") must accept any media - middleware path that has no
// mediaItemID context. Anti-regression for the `mediaItemID != ""` short-
// circuit: a mutation to `==` would reject every middleware lookup.
func TestStreamTokenStore_ValidateEmptyMediaAccepts(t *testing.T) {
	store := media.NewStreamTokenStore(5 * time.Minute)
	defer store.Stop()

	token := store.Create("user-1", "profile-1", "media-1")

	st, ok := store.Validate(token, "")
	require.True(t, ok, "empty mediaItemID should be accepted")
	assert.Equal(t, shared.MediaItemID("media-1"), st.MediaItemID)
}

func TestStreamTokenStore_MultipleMedias(t *testing.T) {
	store := media.NewStreamTokenStore(5 * time.Minute)
	defer store.Stop()

	t1 := store.Create("user-1", "profile-1", "media-1")
	t2 := store.Create("user-1", "profile-1", "media-2")

	st1, ok := store.Validate(t1, "media-1")
	require.True(t, ok)
	assert.Equal(t, shared.MediaItemID("media-1"), st1.MediaItemID)

	st2, ok := store.Validate(t2, "media-2")
	require.True(t, ok)
	assert.Equal(t, shared.MediaItemID("media-2"), st2.MediaItemID)

	// Cross-validate should fail
	_, ok = store.Validate(t1, "media-2")
	assert.False(t, ok)
}

func TestStreamTokenStore_RevokeByUser(t *testing.T) {
	store := media.NewStreamTokenStore(5 * time.Minute)
	defer store.Stop()

	a1 := store.Create("user-1", "profile-1", "media-1")
	a2 := store.CreateWithTTL("user-1", "profile-1", "media-2", 12*time.Hour) // long-lived cast token
	b1 := store.Create("user-2", "profile-2", "media-1")
	guest := store.CreateGuestToken("media-3", "room-3", 5*time.Minute)

	n := store.RevokeByUser("user-1")
	assert.Equal(t, 2, n, "both of user-1's tokens (incl. the 12h cast token) must be revoked")

	if _, ok := store.Validate(a1, "media-1"); ok {
		t.Error("user-1 token a1 should be revoked")
	}
	if _, ok := store.Validate(a2, "media-2"); ok {
		t.Error("user-1 long-lived cast token a2 should be revoked")
	}
	if _, ok := store.Validate(b1, "media-1"); !ok {
		t.Error("user-2 token must survive - only the banned user is purged")
	}
	if _, ok := store.Validate(guest, "media-3"); !ok {
		t.Error("guest token (no user) must survive")
	}

	assert.Equal(t, 0, store.RevokeByUser(""), "empty userID is a no-op")
}

// TestStreamTokenStore_AbsoluteLifetimeCap proves a token cannot be kept alive
// forever by steady traffic sliding its TTL: past the 24h absolute cap from
// creation, Validate must reject it even though each individual renewal
// happened well within its own 12h sliding window.
func TestStreamTokenStore_AbsoluteLifetimeCap(t *testing.T) {
	fakeNow := time.Now()
	store := media.NewStreamTokenStoreForTest(media.CastStreamTokenTTL, func() time.Time { return fakeNow })
	defer store.Stop()

	token := store.CreateWithTTL("user-1", "profile-1", "media-1", media.CastStreamTokenTTL)

	// Renew every 6h, well inside the 12h sliding window each time. After 4
	// renewals exactly 24h has elapsed since creation - the absolute cap
	// caps ExpiresAt at that instant but doesn't reject yet (not strictly past it).
	for range 4 {
		fakeNow = fakeNow.Add(6 * time.Hour)
		_, ok := store.Validate(token, "media-1")
		require.True(t, ok, "renewal within both the sliding window and the absolute cap must succeed")
	}

	// One more tick past the 24h absolute cap: the sliding window (12h) would
	// still allow it, but the absolute lifetime must not.
	fakeNow = fakeNow.Add(1 * time.Hour)
	_, ok := store.Validate(token, "media-1")
	assert.False(t, ok, "token past the absolute lifetime cap must be rejected even mid-sliding-window")
}

// TestStreamTokenStore_NormalTokenSurvivesUnderCap proves the absolute cap
// does not interfere with ordinary tokens well within it.
func TestStreamTokenStore_NormalTokenSurvivesUnderCap(t *testing.T) {
	fakeNow := time.Now()
	store := media.NewStreamTokenStoreForTest(2*time.Hour, func() time.Time { return fakeNow })
	defer store.Stop()

	token := store.Create("user-1", "profile-1", "media-1")

	fakeNow = fakeNow.Add(1 * time.Hour)
	_, ok := store.Validate(token, "media-1")
	assert.True(t, ok, "a token renewed well within the absolute cap must stay valid")
}
