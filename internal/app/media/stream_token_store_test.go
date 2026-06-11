package media_test

import (
	"testing"
	"time"

	"github.com/evey-project/evey/internal/app/media"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestStreamTokenStore_ValidateAny(t *testing.T) {
	store := media.NewStreamTokenStore(5 * time.Minute)
	defer store.Stop()

	token := store.Create("user-1", "profile-1", "media-1")

	st, ok := store.ValidateAny(token)
	require.True(t, ok)
	assert.Equal(t, shared.MediaItemID("media-1"), st.MediaItemID)
}

func TestStreamTokenStore_Expired(t *testing.T) {
	store := media.NewStreamTokenStore(10 * time.Millisecond)
	defer store.Stop()

	token := store.Create("user-1", "profile-1", "media-1")
	time.Sleep(20 * time.Millisecond)

	_, ok := store.Validate(token, "media-1")
	assert.False(t, ok, "expired token should fail")
}

func TestStreamTokenStore_RenewsTTL(t *testing.T) {
	store := media.NewStreamTokenStore(50 * time.Millisecond)
	defer store.Stop()

	token := store.Create("user-1", "profile-1", "media-1")

	// Access at 30ms — should renew to 80ms
	time.Sleep(30 * time.Millisecond)
	_, ok := store.Validate(token, "media-1")
	require.True(t, ok)

	// At 60ms (past original 50ms) — should still be valid due to renewal
	time.Sleep(30 * time.Millisecond)
	_, ok = store.Validate(token, "media-1")
	assert.True(t, ok, "token should be renewed after access")
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

// Validate(token, "") must accept any media — middleware path that has no
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
