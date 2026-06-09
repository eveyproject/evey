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
