package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestPerson(id, name string) *library.Person {
	return &library.Person{
		ID:          id,
		Name:        name,
		ProviderIDs: map[string]string{"tmdb": "t-" + id},
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

	// Same provider id, other uuid: what two concurrent enrichments of one
	// person produce. The id is the identity, the spelling may differ.
	p1 := newTestPerson("pc1", "Bob Jones")
	p2 := &library.Person{ID: "pc2", Name: "Bob M. Jones", ProviderIDs: map[string]string{"tmdb": "t-pc1"}}
	require.NoError(t, repo.Create(ctx, p1))
	require.NoError(t, repo.Create(ctx, p2))

	// RETURNING id: the conflict path must set p2.ID to the SURVIVING row id, not
	// keep p2's own uuid - else the enricher's media_people cast link targets a
	// non-existent person and the FK drops it (concurrent-enrichment race).
	assert.Equal(t, p1.ID, p2.ID, "create conflict must return the existing person id")
	got, err := repo.GetByProviderID(ctx, "tmdb", "t-pc1")
	require.NoError(t, err)
	assert.Equal(t, p1.ID, got.ID)
	assert.Equal(t, "Bob Jones", got.Name, "the stored name is kept")

	// Without a provider id the name stays the identity.
	n1 := &library.Person{ID: "pc3", Name: "Nameless Credit"}
	n2 := &library.Person{ID: "pc4", Name: "Nameless Credit"}
	require.NoError(t, repo.Create(ctx, n1))
	require.NoError(t, repo.Create(ctx, n2))
	assert.Equal(t, n1.ID, n2.ID)
}

// Two people credited under one name but known to the provider under two ids
// are two rows; the old unique name merged them into one.
func TestPersonRepo_Create_HomonymsWithDistinctProviderIDs(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPersonRepository(db)
	ctx := context.Background()

	a := &library.Person{ID: "ph1", Name: "Sam Taylor", ProviderIDs: map[string]string{"tmdb": "101"}}
	b := &library.Person{ID: "ph2", Name: "Sam Taylor", ProviderIDs: map[string]string{"tmdb": "202"}}
	require.NoError(t, repo.Create(ctx, a))
	require.NoError(t, repo.Create(ctx, b))
	assert.Equal(t, "ph1", a.ID)
	assert.Equal(t, "ph2", b.ID)

	got, err := repo.GetByProviderID(ctx, "tmdb", "202")
	require.NoError(t, err)
	assert.Equal(t, "ph2", got.ID)

	_, err = repo.GetByProviderID(ctx, "tmdb", "303")
	require.ErrorIs(t, err, shared.ErrNotFound)

	// A name lookup over homonyms prefers the row WITHOUT an id (the only one
	// a name may claim) and is deterministic otherwise.
	byName, err := repo.GetByName(ctx, "Sam Taylor")
	require.NoError(t, err)
	assert.Equal(t, "ph1", byName.ID)
	idless := &library.Person{ID: "ph3", Name: "Sam Taylor"}
	require.NoError(t, repo.Create(ctx, idless))
	assert.Equal(t, "ph3", idless.ID)
	byName, err = repo.GetByName(ctx, "Sam Taylor")
	require.NoError(t, err)
	assert.Equal(t, "ph3", byName.ID)
	batch, err := repo.GetByNamesBatch(ctx, []string{"Sam Taylor"})
	require.NoError(t, err)
	assert.Equal(t, "ph3", batch["Sam Taylor"].ID)
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

	// ORDER BY name - Aaron < Maria < Zara
	names := make([]string, len(list))
	for i, p := range list {
		names[i] = p.Name
	}
	for i := 1; i < len(names); i++ {
		assert.LessOrEqual(t, names[i-1], names[i], "List should be ordered by name")
	}
}

// Update had no test, and its SET list puts birthday and deathday side by
// side - two strings of the same shape, one positional slot apart. Swap them
// and every person page shows a death date where the birth date belongs, with
// no error anywhere. place_of_birth and known_for are the same story one slot
// further on.
//
// Distinct values everywhere so no pair can hide a swap.
func TestPersonRepo_Update_EveryColumnRoundTripsToItsOwnField(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPersonRepository(db)
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, &library.Person{ID: "p-rt", Name: "Some Person"}))

	thumb := "/thumbs/p-rt.jpg"
	checkedAt := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	updated := &library.Person{
		ID:           "p-rt",
		Name:         "Some Person",
		ProviderIDs:  map[string]string{"tmdb": "777"},
		ThumbPath:    &thumb,
		Biography:    "A biography sentence.",
		Birthday:     "1970-03-04",
		Deathday:     "2019-11-25",
		PlaceOfBirth: "Somewhere",
		KnownFor:     "Acting",
		BioCheckedAt: &checkedAt,
	}
	require.NoError(t, repo.Update(ctx, updated))

	got, err := repo.GetByID(ctx, "p-rt")
	require.NoError(t, err)
	require.NotNil(t, got.ThumbPath)
	assert.Equal(t, thumb, *got.ThumbPath)
	assert.Equal(t, "A biography sentence.", got.Biography)
	// The pair this test exists for.
	assert.Equal(t, "1970-03-04", got.Birthday)
	assert.Equal(t, "2019-11-25", got.Deathday)
	assert.Equal(t, "Somewhere", got.PlaceOfBirth)
	assert.Equal(t, "Acting", got.KnownFor)
	assert.Equal(t, map[string]string{"tmdb": "777"}, got.ProviderIDs)
	require.NotNil(t, got.BioCheckedAt)
	assert.True(t, checkedAt.Equal(*got.BioCheckedAt))
}

func TestPersonRepo_BioCheckedAt_NilByDefaultThenRoundTrips(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPersonRepository(db)
	ctx := context.Background()

	p := newTestPerson("p-bio-cache", "Nil Bio Person")
	require.NoError(t, repo.Create(ctx, p))

	got, err := repo.GetByID(ctx, "p-bio-cache")
	require.NoError(t, err)
	assert.Nil(t, got.BioCheckedAt, "a freshly created person has never been bio-checked")

	checkedAt := time.Date(2026, 9, 1, 8, 30, 0, 0, time.UTC)
	got.BioCheckedAt = &checkedAt
	require.NoError(t, repo.Update(ctx, got))

	reloaded, err := repo.GetByID(ctx, "p-bio-cache")
	require.NoError(t, err)
	require.NotNil(t, reloaded.BioCheckedAt)
	assert.True(t, checkedAt.Equal(*reloaded.BioCheckedAt))
}

// A cast of N names resolves in one query: known names map to their rows
// with provider ids intact, unknown names are simply absent (the caller
// creates them), and an empty list never touches the database.
func TestPersonRepo_GetByNamesBatch(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	repo := postgres.NewPersonRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, newTestPerson("pb1", "Alice Smith")))
	bob := newTestPerson("pb2", "Bob Jones")
	bob.ProviderIDs = map[string]string{"tmdb": "42", "imdb": "nm0000042"}
	require.NoError(t, repo.Create(ctx, bob))
	require.NoError(t, repo.Create(ctx, newTestPerson("pb3", "Carol White")))

	got, err := repo.GetByNamesBatch(ctx, []string{"Alice Smith", "Bob Jones", "Nobody Here"})
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "pb1", got["Alice Smith"].ID)
	assert.Equal(t, "pb2", got["Bob Jones"].ID)
	assert.Equal(t, map[string]string{"tmdb": "42", "imdb": "nm0000042"}, got["Bob Jones"].ProviderIDs)
	assert.NotContains(t, got, "Carol White", "only the names asked for")
	assert.NotContains(t, got, "Nobody Here")

	none, err := repo.GetByNamesBatch(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, none)
}
