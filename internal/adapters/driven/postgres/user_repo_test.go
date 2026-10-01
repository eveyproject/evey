package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestUser(id string) *user.User {
	return &user.User{
		ID:           shared.UserID(id),
		Username:     "user_" + id,
		Email:        id + "@example.com",
		PasswordHash: "hash",
		IsAdmin:      false,
		Enabled:      true,
		CreatedAt:    time.Now().UTC().Truncate(time.Millisecond),
		UpdatedAt:    time.Now().UTC().Truncate(time.Millisecond),
	}
}

func TestUserRepo_Create_GetByID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewUserRepository(db)
	ctx := context.Background()

	u := newTestUser("u1")
	require.NoError(t, repo.Create(ctx, u))

	got, err := repo.GetByID(ctx, "u1")
	require.NoError(t, err)
	assert.Equal(t, u.Username, got.Username)
	assert.Equal(t, u.Email, got.Email)
}

func TestUserRepo_GetByUsername(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewUserRepository(db)
	ctx := context.Background()

	u := newTestUser("u2")
	require.NoError(t, repo.Create(ctx, u))

	got, err := repo.GetByUsername(ctx, u.Username)
	require.NoError(t, err)
	assert.Equal(t, u.ID, got.ID)
}

func TestUserRepo_List(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewUserRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, newTestUser("u3")))
	require.NoError(t, repo.Create(ctx, newTestUser("u4")))

	list, err := repo.List(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(list), 2)
}

func TestUserRepo_Update(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewUserRepository(db)
	ctx := context.Background()

	u := newTestUser("u5")
	require.NoError(t, repo.Create(ctx, u))

	u.Email = "updated@example.com"
	u.IsAdmin = true
	u.UpdatedAt = time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, repo.Update(ctx, u))

	got, err := repo.GetByID(ctx, shared.UserID(u.ID))
	require.NoError(t, err)
	assert.Equal(t, "updated@example.com", got.Email)
	assert.True(t, got.IsAdmin)
}

func TestUserRepo_Delete(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewUserRepository(db)
	ctx := context.Background()

	u := newTestUser("u6")
	require.NoError(t, repo.Create(ctx, u))
	require.NoError(t, repo.Delete(ctx, shared.UserID(u.ID)))

	_, err := repo.GetByID(ctx, shared.UserID(u.ID))
	assert.Error(t, err)
}

func TestUserRepo_CountAdmins(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewUserRepository(db)
	ctx := context.Background()

	u := newTestUser("u7")
	u.IsAdmin = true
	require.NoError(t, repo.Create(ctx, u))

	count, err := repo.CountAdmins(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, 1)
}

func TestUserRepo_CountEnabledAdmins(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewUserRepository(db)
	ctx := context.Background()

	// 1 enabled admin
	enabledAdmin := newTestUser("cea-u1")
	enabledAdmin.IsAdmin = true
	enabledAdmin.Enabled = true
	require.NoError(t, repo.Create(ctx, enabledAdmin))

	// 1 disabled admin
	disabledAdmin := newTestUser("cea-u2")
	disabledAdmin.IsAdmin = true
	disabledAdmin.Enabled = false
	require.NoError(t, repo.Create(ctx, disabledAdmin))

	// 1 non-admin
	nonAdmin := newTestUser("cea-u3")
	nonAdmin.IsAdmin = false
	nonAdmin.Enabled = true
	require.NoError(t, repo.Create(ctx, nonAdmin))

	enabledCount, err := repo.CountEnabledAdmins(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, enabledCount, "CountEnabledAdmins must count only is_admin=TRUE AND enabled=TRUE")

	totalCount, err := repo.CountAdmins(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, totalCount, 2, "CountAdmins must include disabled admins")
}

func TestUserRepo_GetByOIDCSubject(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewUserRepository(db)
	ctx := context.Background()

	issuer := "https://auth.example.com"
	subject := "oidc-sub-123"
	u := newTestUser("u-oidc")
	u.OIDCIssuer = &issuer
	u.OIDCSubject = &subject
	require.NoError(t, repo.Create(ctx, u))

	got, err := repo.GetByOIDCSubject(ctx, issuer, subject)
	require.NoError(t, err)
	assert.Equal(t, u.ID, got.ID)
	assert.Equal(t, &issuer, got.OIDCIssuer)
	assert.Equal(t, &subject, got.OIDCSubject)
}

func TestUserRepo_GetByOIDCSubject_NotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewUserRepository(db)
	ctx := context.Background()

	_, err := repo.GetByOIDCSubject(ctx, "https://nope.com", "no-sub")
	assert.Error(t, err)
}

// LinkOIDCIdentity attaches an SSO identity to a local account, only while it
// has none, and never an identity another account already holds.
func TestUserRepo_LinkOIDCIdentity(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewUserRepository(db)
	ctx := context.Background()
	issuer := "https://auth.example.com"

	local := newTestUser("u-link")
	local.PasswordHash = "keep-me"
	require.NoError(t, repo.Create(ctx, local))
	require.NoError(t, repo.LinkOIDCIdentity(ctx, local.ID, issuer, "sub-1"))

	got, err := repo.GetByOIDCSubject(ctx, issuer, "sub-1")
	require.NoError(t, err)
	assert.Equal(t, local.ID, got.ID)
	assert.Equal(t, "keep-me", got.PasswordHash, "linking must write the identity columns only")

	require.ErrorIs(t, repo.LinkOIDCIdentity(ctx, local.ID, issuer, "sub-2"), shared.ErrConflict,
		"an account already linked must not be re-linked")

	other := newTestUser("u-link-other")
	require.NoError(t, repo.Create(ctx, other))
	require.ErrorIs(t, repo.LinkOIDCIdentity(ctx, other.ID, issuer, "sub-1"), shared.ErrConflict,
		"an identity held by another account must conflict")

	assert.ErrorIs(t, repo.LinkOIDCIdentity(ctx, "usr-missing", issuer, "sub-3"), shared.ErrNotFound)
}

// The partial unique index idx_users_oidc_identity (migration 000113) blocks a
// second user with the SAME (oidc_issuer, oidc_subject) - even with a different
// username - so a concurrent OIDC first-login can't create a duplicate identity.
// Create must surface it as shared.ErrConflict so OIDCService can recover.
func TestUserRepo_Create_DuplicateOIDCIdentity_Conflicts(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewUserRepository(db)
	ctx := context.Background()

	issuer := "https://auth.example.com"
	subject := "shared-sub-xyz"

	first := newTestUser("u-oidc-a") // username user_u-oidc-a
	first.OIDCIssuer = &issuer
	first.OIDCSubject = &subject
	require.NoError(t, repo.Create(ctx, first))

	second := newTestUser("u-oidc-b") // DIFFERENT username, SAME identity
	second.OIDCIssuer = &issuer
	second.OIDCSubject = &subject
	err := repo.Create(ctx, second)
	require.Error(t, err, "duplicate (issuer, subject) must be rejected")
	assert.ErrorIs(t, err, shared.ErrConflict, "must surface as ErrConflict for graceful recovery")
}

// The index is PARTIAL (WHERE oidc_subject IS NOT NULL): the many local users
// with a NULL oidc_subject must coexist freely.
func TestUserRepo_Create_NullOIDCSubject_NoConflict(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewUserRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, newTestUser("u-local-1")))
	require.NoError(t, repo.Create(ctx, newTestUser("u-local-2")),
		"two local users (NULL oidc_subject) must not collide on the partial index")
}

func TestUserRepo_TokenEpoch_GetAndBump(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	repo := postgres.NewUserRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, newTestUser("epoch1")))

	epoch, err := repo.GetTokenEpoch(ctx, "epoch1")
	require.NoError(t, err)
	assert.Equal(t, int64(0), epoch, "fresh user starts at the column default")

	require.NoError(t, repo.BumpTokenEpoch(ctx, "epoch1"))
	require.NoError(t, repo.BumpTokenEpoch(ctx, "epoch1"))

	epoch, err = repo.GetTokenEpoch(ctx, "epoch1")
	require.NoError(t, err)
	assert.Equal(t, int64(2), epoch, "bump is an atomic increment")

	_, err = repo.GetTokenEpoch(ctx, "missing-user")
	require.ErrorIs(t, err, shared.ErrNotFound)
	require.ErrorIs(t, repo.BumpTokenEpoch(ctx, "missing-user"), shared.ErrNotFound)
}

// Migration 000113 must dedup pre-existing duplicate OIDC identities before
// creating the partial unique index, otherwise an instance that already hit the
// race the index guards against fails the migration (23505) and the deploy
// sticks. This reproduces the pre-migration duplicate state and exercises the
// migration's DELETE step + the resulting index creation.
func TestMigration113_DedupesDuplicateOIDCIdentities(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	userRepo := postgres.NewUserRepository(db)

	// Drop the index so we can recreate the duplicate state the migration heals.
	_, err := db.ExecContext(ctx, `DROP INDEX IF EXISTS idx_users_oidc_identity`)
	require.NoError(t, err)

	older := newTestUser("oidc-old")
	newer := newTestUser("oidc-new")
	require.NoError(t, userRepo.Create(ctx, older))
	require.NoError(t, userRepo.Create(ctx, newer))
	// Same federated identity, older created earlier.
	_, err = db.ExecContext(ctx,
		`UPDATE users SET oidc_issuer='iss', oidc_subject='sub', created_at='2020-01-01' WHERE id=$1`, string(older.ID))
	require.NoError(t, err)
	_, err = db.ExecContext(ctx,
		`UPDATE users SET oidc_issuer='iss', oidc_subject='sub', created_at='2020-06-01' WHERE id=$1`, string(newer.ID))
	require.NoError(t, err)

	// The migration's dedup step (keep the oldest).
	_, err = db.ExecContext(ctx, `
		DELETE FROM users a USING users b
		 WHERE a.oidc_subject IS NOT NULL
		   AND a.oidc_issuer = b.oidc_issuer
		   AND a.oidc_subject = b.oidc_subject
		   AND (a.created_at > b.created_at OR (a.created_at = b.created_at AND a.id > b.id))`)
	require.NoError(t, err)

	_, err = userRepo.GetByID(ctx, older.ID)
	require.NoError(t, err, "oldest user must be kept")
	_, err = userRepo.GetByID(ctx, newer.ID)
	require.Error(t, err, "newer duplicate must be removed")

	// The unique index is now creatable (no remaining duplicates).
	_, err = db.ExecContext(ctx,
		`CREATE UNIQUE INDEX idx_users_oidc_identity ON users (oidc_issuer, oidc_subject) WHERE oidc_subject IS NOT NULL`)
	require.NoError(t, err, "index must be creatable after dedup")
}

// Same reasoning as TestProfileRepo_Update_EveryColumnRoundTripsToItsOwnField:
// 13 hand-enumerated SET columns, several of them interchangeable in type and
// adjacent (max_streams / max_profiles / max_devices_per_profile are three
// *int in a row, and push_consent sits next to the bitrate cap). A positional
// shift compiles, writes, reports one row affected, and silently stores a
// stream cap as a profile cap.
//
// Distinct non-zero values everywhere: two fields sharing a value cannot
// detect a swap between them.
func TestUserRepo_Update_EveryColumnRoundTripsToItsOwnField(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewUserRepository(db)
	ctx := context.Background()
	u := newTestUser("u-rt")
	require.NoError(t, repo.Create(ctx, u))

	sub, iss := "oidc-subject-1", "https://issuer.example"
	streams, profiles, devices := 3, 5, 7
	u.Username = "renamed_user"
	u.Email = "renamed@example.com"
	u.PasswordHash = "a-different-hash"
	u.IsAdmin = true
	u.Enabled = false
	u.OIDCSubject = &sub
	u.OIDCIssuer = &iss
	u.MaxStreams = &streams
	u.MaxProfiles = &profiles
	u.MaxDevicesPerProfile = &devices
	u.MaxBitrateKbps = 9876
	u.PushConsent = true
	u.UpdatedAt = time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, repo.Update(ctx, u))

	got, err := repo.GetByID(ctx, u.ID)
	require.NoError(t, err)
	assert.Equal(t, "renamed_user", got.Username)
	assert.Equal(t, "renamed@example.com", got.Email)
	assert.Equal(t, "a-different-hash", got.PasswordHash)
	assert.True(t, got.IsAdmin)
	assert.False(t, got.Enabled)
	require.NotNil(t, got.OIDCSubject)
	assert.Equal(t, sub, *got.OIDCSubject)
	require.NotNil(t, got.OIDCIssuer)
	assert.Equal(t, iss, *got.OIDCIssuer)
	// The three adjacent *int caps - the group a shift would scramble.
	require.NotNil(t, got.MaxStreams)
	assert.Equal(t, 3, *got.MaxStreams)
	require.NotNil(t, got.MaxProfiles)
	assert.Equal(t, 5, *got.MaxProfiles)
	require.NotNil(t, got.MaxDevicesPerProfile)
	assert.Equal(t, 7, *got.MaxDevicesPerProfile)
	assert.Equal(t, 9876, got.MaxBitrateKbps)
	assert.True(t, got.PushConsent)
}

// Lookup by e-mail is the password-reset entry point; TouchLastLogin stamps
// the account without touching anything else.
func TestUserRepo_GetByEmail_TouchLastLogin(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	repo := postgres.NewUserRepository(db)
	ctx := context.Background()
	u := newTestUser("ue-1")
	require.NoError(t, repo.Create(ctx, u))

	got, err := repo.GetByEmail(ctx, "ue-1@example.com")
	require.NoError(t, err)
	assert.Equal(t, u.ID, got.ID)
	_, err = repo.GetByEmail(ctx, "nobody@example.com")
	require.Error(t, err)
	_, err = repo.GetByEmail(ctx, "UE-1@example.com")
	require.Error(t, err, "the column match is exact; normalisation is the caller's job")

	at := time.Date(2026, 9, 16, 3, 0, 0, 0, time.UTC)
	require.NoError(t, repo.TouchLastLogin(ctx, u.ID, at))
	got, err = repo.GetByID(ctx, u.ID)
	require.NoError(t, err)
	require.NotNil(t, got.LastLogin)
	assert.True(t, got.LastLogin.Equal(at))
	assert.Equal(t, u.Username, got.Username)
}

// Login by username is case-insensitive (prod 2026-09: viewers typed their
// own name in lowercase and were told the account did not exist), and the
// lower(username) unique index refuses the pair that would make it ambiguous.
func TestUserRepo_GetByUsername_CaseInsensitive_AndUniqueUnderLower(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewUserRepository(db)
	ctx := context.Background()

	u := newTestUser("u-case")
	u.Username = "Feregoto"
	require.NoError(t, repo.Create(ctx, u))

	got, err := repo.GetByUsername(ctx, "feregoto")
	require.NoError(t, err)
	assert.Equal(t, u.ID, got.ID)
	got, err = repo.GetByUsername(ctx, "FEREGOTO")
	require.NoError(t, err)
	assert.Equal(t, u.ID, got.ID)

	dup := newTestUser("u-case-2")
	dup.Username = "feregoto"
	err = repo.Create(ctx, dup)
	require.Error(t, err, "a second account differing only by case must be refused")
	assert.ErrorIs(t, err, shared.ErrConflict)
}
