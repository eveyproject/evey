package postgres_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/eveyproject/evey/internal/domain/shared"
)

func TestTrustedPublisherRepo_UpsertGetListDelete(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	repo := postgres.NewTrustedPublisherRepository(db)
	ctx := context.Background()

	_, err := repo.Get(ctx, "sha256:aa")
	require.ErrorIs(t, err, shared.ErrNotFound)

	require.NoError(t, repo.Upsert(ctx, plugin.TrustedPublisher{Fingerprint: "sha256:aa", Name: "Acme", PublicKeyPEM: "pem-a"}))
	require.NoError(t, repo.Upsert(ctx, plugin.TrustedPublisher{Fingerprint: "sha256:bb", Name: "Beta", PublicKeyPEM: "pem-b"}))

	got, err := repo.Get(ctx, "sha256:aa")
	require.NoError(t, err)
	assert.Equal(t, "Acme", got.Name)
	assert.Equal(t, "pem-a", got.PublicKeyPEM)
	assert.False(t, got.Seeded)
	assert.False(t, got.AddedAt.IsZero())
	firstAdded := got.AddedAt

	// Renaming keeps the original added_at.
	require.NoError(t, repo.Upsert(ctx, plugin.TrustedPublisher{Fingerprint: "sha256:aa", Name: "Acme Inc", PublicKeyPEM: "pem-a"}))
	got, err = repo.Get(ctx, "sha256:aa")
	require.NoError(t, err)
	assert.Equal(t, "Acme Inc", got.Name)
	assert.True(t, got.AddedAt.Equal(firstAdded))

	list, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "sha256:aa", list[0].Fingerprint)

	require.NoError(t, repo.Delete(ctx, "sha256:bb"))
	require.ErrorIs(t, repo.Delete(ctx, "sha256:bb"), shared.ErrNotFound)
	list, err = repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 1)
}

func TestTrustedPublisherRepo_SeededCannotBeDeleted(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	repo := postgres.NewTrustedPublisherRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.Upsert(ctx, plugin.TrustedPublisher{Fingerprint: "sha256:op", Name: "Server operator key", PublicKeyPEM: "pem", Seeded: true}))
	require.ErrorIs(t, repo.Delete(ctx, "sha256:op"), plugin.ErrSeededPublisher)
	_, err := repo.Get(ctx, "sha256:op")
	require.NoError(t, err, "a refused delete leaves the row")

	// Re-adding a seeded key by hand does not make it removable.
	require.NoError(t, repo.Upsert(ctx, plugin.TrustedPublisher{Fingerprint: "sha256:op", Name: "Renamed", PublicKeyPEM: "pem"}))
	got, err := repo.Get(ctx, "sha256:op")
	require.NoError(t, err)
	assert.True(t, got.Seeded)
	require.ErrorIs(t, repo.Delete(ctx, "sha256:op"), plugin.ErrSeededPublisher)
}

func TestTrustedPublisherRepo_PruneSeeded(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	repo := postgres.NewTrustedPublisherRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.Upsert(ctx, plugin.TrustedPublisher{Fingerprint: "sha256:old", Name: "old", PublicKeyPEM: "p", Seeded: true}))
	require.NoError(t, repo.Upsert(ctx, plugin.TrustedPublisher{Fingerprint: "sha256:cur", Name: "cur", PublicKeyPEM: "p", Seeded: true}))
	require.NoError(t, repo.Upsert(ctx, plugin.TrustedPublisher{Fingerprint: "sha256:hand", Name: "hand", PublicKeyPEM: "p"}))

	require.NoError(t, repo.PruneSeeded(ctx, []string{"sha256:cur"}))
	list, err := repo.List(ctx)
	require.NoError(t, err)
	var fps []string
	for _, p := range list {
		fps = append(fps, p.Fingerprint)
	}
	assert.ElementsMatch(t, []string{"sha256:cur", "sha256:hand"}, fps)

	require.NoError(t, repo.PruneSeeded(ctx, nil))
	list, err = repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "sha256:hand", list[0].Fingerprint)
}
