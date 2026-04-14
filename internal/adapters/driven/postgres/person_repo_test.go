package postgres_test

import (
	"context"
	"testing"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestPerson(id, name string) *library.Person {
	return &library.Person{
		ID:          id,
		Name:        name,
		ProviderIDs: map[string]string{"tmdb": "999"},
		ThumbPath:   nil,
	}
}

func TestPersonRepo_Create_basic(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPersonRepository(db)
	ctx := context.Background()

	p := newTestPerson("p1", "Alice Smith")
	require.NoError(t, repo.Create(ctx, p))
}

func TestPersonRepo_Create_Conflict(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPersonRepository(db)
	ctx := context.Background()

	p1 := newTestPerson("pc1", "Bob Jones")
	p2 := newTestPerson("pc2", "Bob Jones") // same name, different id
	require.NoError(t, repo.Create(ctx, p1))
	require.NoError(t, repo.Create(ctx, p2)) // ON CONFLICT DO NOTHING — no error
}

func TestPersonRepo_GetByID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPersonRepository(db)
	ctx := context.Background()

	thumb := "/thumbs/carol.jpg"
	p := &library.Person{
		ID:          "pg1",
		Name:        "Carol White",
		ProviderIDs: map[string]string{"tmdb": "42", "imdb": "nm0001"},
		ThumbPath:   &thumb,
	}
	require.NoError(t, repo.Create(ctx, p))

	got, err := repo.GetByID(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, p.ID, got.ID)
	assert.Equal(t, p.Name, got.Name)
	assert.Equal(t, p.ProviderIDs, got.ProviderIDs)
	require.NotNil(t, got.ThumbPath)
	assert.Equal(t, thumb, *got.ThumbPath)
}

func TestPersonRepo_GetByID_NotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPersonRepository(db)
	_, err := repo.GetByID(context.Background(), "nonexistent")
	assert.ErrorIs(t, err, shared.ErrNotFound)
}

func TestPersonRepo_GetByName(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPersonRepository(db)
	ctx := context.Background()

	p := newTestPerson("pn1", "Dave Brown")
	require.NoError(t, repo.Create(ctx, p))

	got, err := repo.GetByName(ctx, "Dave Brown")
	require.NoError(t, err)
	assert.Equal(t, p.ID, got.ID)
	assert.Equal(t, "Dave Brown", got.Name)
}

func TestPersonRepo_GetByName_NotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPersonRepository(db)
	_, err := repo.GetByName(context.Background(), "Nobody Here")
	assert.ErrorIs(t, err, shared.ErrNotFound)
}

func TestPersonRepo_List(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPersonRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, newTestPerson("pl1", "Zara Fox")))
	require.NoError(t, repo.Create(ctx, newTestPerson("pl2", "Aaron Grant")))
	require.NoError(t, repo.Create(ctx, newTestPerson("pl3", "Maria Hill")))

	list, err := repo.List(ctx)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(list), 3)

	// ORDER BY name — Aaron < Maria < Zara
	names := make([]string, len(list))
	for i, p := range list {
		names[i] = p.Name
	}
	for i := 1; i < len(names); i++ {
		assert.LessOrEqual(t, names[i-1], names[i], "List should be ordered by name")
	}
}
