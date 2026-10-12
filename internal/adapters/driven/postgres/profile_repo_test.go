package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
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

	// GetAll (the admin session panel's label map) spans every account, the
	// default profile of each account first.
	u2 := newTestUser("pu2b")
	require.NoError(t, userRepo.Create(ctx, u2))
	def := newTestProfile("p2c", u2.ID)
	def.IsDefault = true
	require.NoError(t, profileRepo.Create(ctx, newTestProfile("p2d", u2.ID)))
	require.NoError(t, profileRepo.Create(ctx, def))
	all, err := profileRepo.GetAll(ctx)
	require.NoError(t, err)
	byUser := map[string][]string{}
	for _, p := range all {
		byUser[string(p.UserID)] = append(byUser[string(p.UserID)], string(p.ID))
	}
	assert.ElementsMatch(t, []string{"p2a", "p2b"}, byUser[string(u.ID)])
	assert.Equal(t, []string{"p2c", "p2d"}, byUser[string(u2.ID)], "the default profile leads its account")
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

// SetDefault moves the one default flag of an account in a single
// transaction, refuses a profile of another account (and rolls the clear
// back, so the account keeps its default), ListAdminProfiles is what pages
// admins only, and TouchLastActive stamps the profile.
func TestProfileRepo_SetDefault_ListAdminProfiles_TouchLastActive(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	userRepo := postgres.NewUserRepository(db)
	repo := postgres.NewProfileRepository(db)
	admin := newTestUser("pd-admin")
	admin.IsAdmin = true
	require.NoError(t, userRepo.Create(ctx, admin))
	member := newTestUser("pd-member")
	require.NoError(t, userRepo.Create(ctx, member))
	a1, a2 := newTestProfile("pd-a1", admin.ID), newTestProfile("pd-a2", admin.ID)
	a1.IsDefault = true
	m1 := newTestProfile("pd-m1", member.ID)
	m1.IsDefault = true
	for _, p := range []*user.Profile{a1, a2, m1} {
		require.NoError(t, repo.Create(ctx, p))
	}

	require.NoError(t, repo.SetDefault(ctx, admin.ID, a2.ID))
	got, err := repo.GetByUserID(ctx, admin.ID)
	require.NoError(t, err)
	defaults := map[shared.ProfileID]bool{}
	for _, p := range got {
		defaults[p.ID] = p.IsDefault
	}
	assert.Equal(t, map[shared.ProfileID]bool{a1.ID: false, a2.ID: true}, defaults, "exactly one default after the move")

	// Another account's profile: refused, and the admin keeps its default.
	require.ErrorIs(t, repo.SetDefault(ctx, admin.ID, m1.ID), shared.ErrNotFound)
	got, _ = repo.GetByUserID(ctx, admin.ID)
	for _, p := range got {
		if p.ID == a2.ID {
			assert.True(t, p.IsDefault, "the rolled-back attempt must not have cleared the default")
		}
	}
	mp, err := repo.GetByID(ctx, m1.ID)
	require.NoError(t, err)
	assert.True(t, mp.IsDefault, "the other account is untouched")

	admins, err := repo.ListAdminProfiles(ctx)
	require.NoError(t, err)
	ids := make([]shared.ProfileID, 0, len(admins))
	for _, p := range admins {
		ids = append(ids, p.ID)
	}
	assert.Equal(t, []shared.ProfileID{a2.ID, a1.ID}, ids, "admin profiles only, default first")

	at := time.Date(2026, 9, 16, 3, 0, 0, 0, time.UTC)
	require.NoError(t, repo.TouchLastActive(ctx, m1.ID, at))
	mp, err = repo.GetByID(ctx, m1.ID)
	require.NoError(t, err)
	require.NotNil(t, mp.LastActiveAt)
	assert.True(t, mp.LastActiveAt.Equal(at))
}

// Opening hours round-trip through every read, are written column-scoped (a
// full Update from a stale snapshot does not erase them), and a stored value
// that no longer decodes keeps the profile CLOSED, never open.
func TestProfileRepo_AccessSchedule_RoundTripScopedAndFailClosed(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	ctx := context.Background()

	u := newTestUser("sched-u")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("sched-p", u.ID)
	p.IsKids = true
	require.NoError(t, profileRepo.Create(ctx, p))

	got, err := profileRepo.GetByID(ctx, p.ID)
	require.NoError(t, err)
	assert.Nil(t, got.Schedule, "no opening hours until a parent sets them")

	s := user.DefaultKidsSchedule()
	require.NoError(t, profileRepo.UpdateSchedule(ctx, p.ID, &s))
	until := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Microsecond)
	require.NoError(t, profileRepo.SetScheduleOverride(ctx, p.ID, &until))

	got, err = profileRepo.GetByID(ctx, p.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Schedule)
	assert.Equal(t, s, *got.Schedule)
	require.NotNil(t, got.ScheduleOverrideUntil)
	assert.True(t, until.Equal(*got.ScheduleOverrideUntil))

	list, err := profileRepo.GetByUserID(ctx, u.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, s, *list[0].Schedule, "the list read carries the schedule too")

	stale := newTestProfile("sched-p", u.ID)
	stale.IsKids = true
	stale.Name = "renamed"
	require.NoError(t, profileRepo.Update(ctx, stale))
	got, err = profileRepo.GetByID(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, s, *got.Schedule, "a full-row update must not erase the opening hours")

	_, err = db.ExecContext(ctx, `UPDATE profiles SET access_schedule = '{"enabled":true,"mode":"week","rules":[{"days":[1],"ranges":[{"start":60,"end":60}]}]}' WHERE id = $1`, string(p.ID))
	require.NoError(t, err)
	got, err = profileRepo.GetByID(ctx, p.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Schedule)
	assert.True(t, got.Schedule.Enabled)
	assert.Empty(t, got.Schedule.Rules, "an invalid stored schedule reads as closed, not as no schedule")

	require.NoError(t, profileRepo.UpdateSchedule(ctx, p.ID, nil))
	require.NoError(t, profileRepo.SetScheduleOverride(ctx, p.ID, nil))
	got, err = profileRepo.GetByID(ctx, p.ID)
	require.NoError(t, err)
	assert.Nil(t, got.Schedule)
	assert.Nil(t, got.ScheduleOverrideUntil)
}
