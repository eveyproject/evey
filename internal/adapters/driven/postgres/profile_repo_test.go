package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestProfile(id string, userID shared.UserID) *user.Profile {
	return &user.Profile{
		ID:          shared.ProfileID(id),
		UserID:      userID,
		Name:        "Profile " + id,
		Preferences: map[string]string{"lang": "en"},
		IsDefault:   false,
		CreatedAt:   time.Now().UTC().Truncate(time.Millisecond),
	}
}

// TestProfileRepo_CreateWithinCap_Concurrent proves the per-user cap holds under
// concurrent creates (the F2 TOCTOU: count-then-create let requests overshoot).
func TestProfileRepo_CreateWithinCap_Concurrent(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	ctx := context.Background()

	u := newTestUser("capu")
	require.NoError(t, userRepo.Create(ctx, u))

	const maxCap = 2
	const attempts = 12
	var okCount, capCount int64
	var wg sync.WaitGroup
	for i := range attempts {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p := newTestProfile(fmt.Sprintf("capp-%d", i), u.ID)
			switch err := profileRepo.CreateWithinCap(ctx, p, maxCap); {
			case err == nil:
				atomic.AddInt64(&okCount, 1)
			case errors.Is(err, user.ErrProfileCapReached):
				atomic.AddInt64(&capCount, 1)
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}(i)
	}
	wg.Wait()

	assert.Equal(t, int64(maxCap), okCount, "exactly maxCap creates succeed")
	assert.Equal(t, int64(attempts-maxCap), capCount, "the rest hit the cap")

	got, err := profileRepo.GetByUserID(ctx, u.ID)
	require.NoError(t, err)
	assert.Len(t, got, maxCap, "DB must hold exactly maxCap profiles")
}

// CreateWithinCap with maxProfiles<=0 is unlimited.
func TestProfileRepo_CreateWithinCap_Unlimited(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	ctx := context.Background()

	u := newTestUser("uncap")
	require.NoError(t, userRepo.Create(ctx, u))

	for i := range 5 {
		require.NoError(t, profileRepo.CreateWithinCap(ctx, newTestProfile(fmt.Sprintf("unl-%d", i), u.ID), 0))
	}
	got, err := profileRepo.GetByUserID(ctx, u.ID)
	require.NoError(t, err)
	assert.Len(t, got, 5)
}

func TestProfileRepo_Create_GetByID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	ctx := context.Background()

	u := newTestUser("pu1")
	require.NoError(t, userRepo.Create(ctx, u))

	p := newTestProfile("p1", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))

	got, err := profileRepo.GetByID(ctx, shared.ProfileID(p.ID))
	require.NoError(t, err)
	assert.Equal(t, p.Name, got.Name)
	assert.Equal(t, p.UserID, got.UserID)
	assert.Equal(t, p.Preferences, got.Preferences)
}

func TestProfileRepo_GetByUserID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	ctx := context.Background()

	u := newTestUser("pu2")
	require.NoError(t, userRepo.Create(ctx, u))

	require.NoError(t, profileRepo.Create(ctx, newTestProfile("p2a", u.ID)))
	require.NoError(t, profileRepo.Create(ctx, newTestProfile("p2b", u.ID)))

	profiles, err := profileRepo.GetByUserID(ctx, shared.UserID(u.ID))
	require.NoError(t, err)
	assert.Len(t, profiles, 2)
}

func TestProfileRepo_Update(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	ctx := context.Background()

	u := newTestUser("pu3")
	require.NoError(t, userRepo.Create(ctx, u))

	p := newTestProfile("p3", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))

	p.Name = "Updated Profile"
	p.IsDefault = true
	require.NoError(t, profileRepo.Update(ctx, p))

	got, err := profileRepo.GetByID(ctx, shared.ProfileID(p.ID))
	require.NoError(t, err)
	assert.Equal(t, "Updated Profile", got.Name)
	assert.True(t, got.IsDefault)
}

func TestProfileRepo_LanguagePreferences(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	ctx := context.Background()

	u := newTestUser("pu5")
	require.NoError(t, userRepo.Create(ctx, u))

	p := newTestProfile("p5", u.ID)
	p.AudioLanguages = []string{"fr", "en"}
	p.SubtitleLanguages = []string{"fr"}
	require.NoError(t, profileRepo.Create(ctx, p))

	got, err := profileRepo.GetByID(ctx, shared.ProfileID(p.ID))
	require.NoError(t, err)
	assert.Equal(t, []string{"fr", "en"}, got.AudioLanguages)
	assert.Equal(t, []string{"fr"}, got.SubtitleLanguages)

	got.AudioLanguages = []string{"de", "fr", "en"}
	require.NoError(t, profileRepo.Update(ctx, got))

	updated, err := profileRepo.GetByID(ctx, shared.ProfileID(p.ID))
	require.NoError(t, err)
	assert.Equal(t, []string{"de", "fr", "en"}, updated.AudioLanguages)
}

func TestProfileRepo_Delete(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	ctx := context.Background()

	u := newTestUser("pu4")
	require.NoError(t, userRepo.Create(ctx, u))

	p := newTestProfile("p4", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	require.NoError(t, profileRepo.Delete(ctx, shared.ProfileID(p.ID)))

	_, err := profileRepo.GetByID(ctx, shared.ProfileID(p.ID))
	assert.Error(t, err)
}

// Every column of the 15-column UPDATE must land in ITS OWN field.
//
// profile_repo's Update is the longest hand-enumerated SET in the repo, and
// four of its columns are adjacent and interchangeable in type:
// parental_max_age, parental_blocked_tags, parental_pin_hash,
// parental_enabled. Swap two positional arguments there and the code compiles,
// the write succeeds, RowsAffected is 1, and a PIN lands in the wrong column -
// silently, on a security-relevant field. The existing Update test asserts two
// fields (Name, IsDefault), so a shift anywhere in the other thirteen was
// invisible.
//
// Every value below is DISTINCT and non-zero on purpose: a round-trip where
// two fields share a value cannot detect a swap between them.
func TestProfileRepo_Update_EveryColumnRoundTripsToItsOwnField(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	ctx := context.Background()

	u := newTestUser("pu-rt")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("p-rt", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))

	avatar := "/avatars/p-rt.webp"
	maxRating := "PG-13"
	maxBitrate := 4321
	maxAge := 12
	region := "BE"
	p.Name = "Round Trip"
	p.AvatarPath = &avatar
	p.Preferences = map[string]string{"subtitleSize": "large"}
	p.MaxRating = &maxRating
	p.IsDefault = true
	p.IsKids = true
	p.AudioLanguages = []string{"fr", "de"}
	p.SubtitleLanguages = []string{"es", "hi"}
	p.MaxBitrateKbps = &maxBitrate
	p.Parental = &user.ParentalSettings{
		MaxAge:      &maxAge,
		BlockedTags: []string{"horror", "gore"},
		PINHash:     "hash-of-a-pin",
		Enabled:     true,
	}
	p.Locale = "fr"
	p.Region = &region
	require.NoError(t, profileRepo.Update(ctx, p))

	got, err := profileRepo.GetByID(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, "Round Trip", got.Name)
	require.NotNil(t, got.AvatarPath)
	assert.Equal(t, avatar, *got.AvatarPath)
	assert.Equal(t, map[string]string{"subtitleSize": "large"}, got.Preferences)
	require.NotNil(t, got.MaxRating)
	assert.Equal(t, "PG-13", *got.MaxRating)
	assert.True(t, got.IsDefault)
	assert.True(t, got.IsKids)
	assert.Equal(t, []string{"fr", "de"}, got.AudioLanguages)
	assert.Equal(t, []string{"es", "hi"}, got.SubtitleLanguages)
	require.NotNil(t, got.MaxBitrateKbps)
	assert.Equal(t, 4321, *got.MaxBitrateKbps)
	assert.Equal(t, "fr", got.Locale)
	require.NotNil(t, got.Region)
	assert.Equal(t, "BE", *got.Region)

	// The four adjacent parental columns, each asserted separately - this is
	// the group a positional shift would scramble.
	require.NotNil(t, got.Parental)
	require.NotNil(t, got.Parental.MaxAge)
	assert.Equal(t, 12, *got.Parental.MaxAge)
	assert.Equal(t, []string{"horror", "gore"}, got.Parental.BlockedTags)
	assert.Equal(t, "hash-of-a-pin", got.Parental.PINHash)
	assert.True(t, got.Parental.Enabled)
}

// The two narrow writers must not touch each other's columns.
//
// The comment above UpdateAvatarPath documents the bug this replaced: the
// avatar upload flow re-saved the WHOLE row from a Profile it had read
// earlier, so a parental PIN set by a concurrent request in between was
// silently erased. Scoping each write to the columns that actually changed is
// what makes that clobber structurally impossible - and nothing asserted it,
// so a future "let's just call Update()" refactor would reintroduce it
// silently, on a security-relevant field.
//
// This is the interleaving that used to lose data: read profile, someone else
// sets a PIN, then write the avatar.
func TestProfileRepo_AvatarAndParentalWritesDoNotClobberEachOther(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	ctx := context.Background()

	u := newTestUser("pu-iso")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("p-iso", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))

	// Another request sets a parental PIN.
	maxAge := 10
	require.NoError(t, profileRepo.UpdateParentalSettings(ctx, p.ID, &user.ParentalSettings{
		MaxAge:      &maxAge,
		BlockedTags: []string{"horror"},
		PINHash:     "pin-set-by-someone-else",
		Enabled:     true,
	}))

	// The avatar upload lands afterwards, carrying its own stale view.
	avatar := "/avatars/p-iso.webp"
	require.NoError(t, profileRepo.UpdateAvatarPath(ctx, p.ID, &avatar))

	got, err := profileRepo.GetByID(ctx, p.ID)
	require.NoError(t, err)
	require.NotNil(t, got.AvatarPath)
	assert.Equal(t, avatar, *got.AvatarPath, "the avatar write must land")
	require.NotNil(t, got.Parental, "the avatar write must not erase the parental settings")
	assert.Equal(t, "pin-set-by-someone-else", got.Parental.PINHash)
	assert.True(t, got.Parental.Enabled)
	require.NotNil(t, got.Parental.MaxAge)
	assert.Equal(t, 10, *got.Parental.MaxAge)

	// And the reverse order: a parental write must not drop the avatar.
	require.NoError(t, profileRepo.UpdateParentalSettings(ctx, p.ID, &user.ParentalSettings{
		PINHash: "a-newer-pin", Enabled: true,
	}))
	got2, err := profileRepo.GetByID(ctx, p.ID)
	require.NoError(t, err)
	require.NotNil(t, got2.AvatarPath, "the parental write must not erase the avatar")
	assert.Equal(t, avatar, *got2.AvatarPath)
	assert.Equal(t, "a-newer-pin", got2.Parental.PINHash)
}
