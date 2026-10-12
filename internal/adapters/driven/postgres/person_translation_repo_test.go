package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPersonTranslationRepo_Upsert_Get(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	personRepo := postgres.NewPersonRepository(db)
	repo := postgres.NewPersonTranslationRepository(db)
	ctx := context.Background()

	p := newTestPerson("pt-p1", "Alice")
	require.NoError(t, personRepo.Create(ctx, p))

	tr := &library.PersonTranslation{PersonID: p.ID, Locale: "fr-FR", Biography: "Bio FR"}
	require.NoError(t, repo.Upsert(ctx, tr))

	got, err := repo.GetByPersonAndLocale(ctx, p.ID, "fr-FR")
	require.NoError(t, err)
	assert.Equal(t, "Bio FR", got.Biography)

	require.NoError(t, repo.Upsert(ctx, &library.PersonTranslation{PersonID: p.ID, Locale: "fr-FR", Biography: "Bio FR v2"}))
	got2, err := repo.GetByPersonAndLocale(ctx, p.ID, "fr-FR")
	require.NoError(t, err)
	assert.Equal(t, "Bio FR v2", got2.Biography)
}

func TestPersonTranslationRepo_GetByPersonAndLocale_NotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPersonTranslationRepository(db)
	_, err := repo.GetByPersonAndLocale(context.Background(), "no-such", "en-US")
	assert.True(t, errors.Is(err, shared.ErrNotFound))
}

func TestPersonTranslationRepo_GetByPerson_OrderedAndDelete(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	personRepo := postgres.NewPersonRepository(db)
	repo := postgres.NewPersonTranslationRepository(db)
	ctx := context.Background()

	p := newTestPerson("pt-p2", "Bob")
	require.NoError(t, personRepo.Create(ctx, p))

	for _, loc := range []string{"fr-FR", "en-US", "de-DE"} {
		require.NoError(t, repo.Upsert(ctx, &library.PersonTranslation{PersonID: p.ID, Locale: loc, Biography: "bio-" + loc}))
	}

	list, err := repo.GetByPerson(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, list, 3)
	assert.Equal(t, "de-DE", list[0].Locale)
	assert.Equal(t, "en-US", list[1].Locale)
	assert.Equal(t, "fr-FR", list[2].Locale)

	require.NoError(t, repo.DeleteByPerson(ctx, p.ID))
	list2, err := repo.GetByPerson(ctx, p.ID)
	require.NoError(t, err)
	assert.Empty(t, list2)
}
