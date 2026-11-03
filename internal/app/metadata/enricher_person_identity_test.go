package metadata

import (
	"context"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainlib "github.com/eveyproject/evey/internal/domain/library"
	domain "github.com/eveyproject/evey/internal/domain/metadata"
	"github.com/eveyproject/evey/internal/domain/shared"
)

func enrichCast(t *testing.T, e *Enricher, itemID string, people ...domain.PersonResult) {
	t.Helper()
	item := &domainlib.MediaItem{ID: shared.MediaItemID(itemID), Title: itemID, Type: "movie"}
	require.NoError(t, e.EnrichItem(context.Background(), item, &domain.MetadataResult{Title: itemID, People: people}))
}

func personNamesAndIDs(t *testing.T, list []*domainlib.Person) []string {
	t.Helper()
	out := make([]string, 0, len(list))
	for _, p := range list {
		out = append(out, p.Name+"/"+p.ProviderIDs["tmdb"])
	}
	sort.Strings(out)
	return out
}

// A name is not an identity: two actors credited under the same name but
// known to the provider under two ids are two people, each keeping its own
// biography and filmography. Merging them on the name was the old behaviour.
func TestEnrichItem_HomonymsWithDifferentProviderIDs_StayTwoPeople(t *testing.T) {
	e, _, persons, _, miRepo, _ := newTestEnricher(&fakeProvider{name: "tmdb"}, nil, nil)
	ctx := context.Background()
	require.NoError(t, miRepo.Create(ctx, &domainlib.MediaItem{ID: "item-a", Type: "movie"}))
	require.NoError(t, miRepo.Create(ctx, &domainlib.MediaItem{ID: "item-b", Type: "movie"}))

	enrichCast(t, e, "item-a", domain.PersonResult{Name: "Sam Taylor", Role: "Actor", ProviderID: "101"})
	enrichCast(t, e, "item-b", domain.PersonResult{Name: "Sam Taylor", Role: "Actor", ProviderID: "202"})

	list, err := persons.List(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"Sam Taylor/101", "Sam Taylor/202"}, personNamesAndIDs(t, list))
}

// The same homonym pair inside one cast must not be collapsed by the
// per-cast dedup either.
func TestEnrichItem_HomonymsInOneCast_StayTwoPeople(t *testing.T) {
	e, _, persons, mp, miRepo, _ := newTestEnricher(&fakeProvider{name: "tmdb"}, nil, nil)
	ctx := context.Background()
	require.NoError(t, miRepo.Create(ctx, &domainlib.MediaItem{ID: "item-a", Type: "movie"}))

	enrichCast(t, e, "item-a",
		domain.PersonResult{Name: "Sam Taylor", Role: "Actor", ProviderID: "101"},
		domain.PersonResult{Name: "Sam Taylor", Role: "Director", ProviderID: "202"},
	)

	list, err := persons.List(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 2)
	links, err := mp.GetByMediaItemID(ctx, "item-a")
	require.NoError(t, err)
	assert.Len(t, links, 2)
}

// The provider id is the identity: a name that changed spelling between two
// credits still lands on the one person the id names.
func TestEnrichItem_SameProviderIDDifferentSpelling_IsOnePerson(t *testing.T) {
	e, _, persons, _, miRepo, _ := newTestEnricher(&fakeProvider{name: "tmdb"}, nil, nil)
	ctx := context.Background()
	require.NoError(t, miRepo.Create(ctx, &domainlib.MediaItem{ID: "item-a", Type: "movie"}))
	require.NoError(t, miRepo.Create(ctx, &domainlib.MediaItem{ID: "item-b", Type: "movie"}))

	enrichCast(t, e, "item-a", domain.PersonResult{Name: "Rene Martin", Role: "Actor", ProviderID: "8691"})
	enrichCast(t, e, "item-b", domain.PersonResult{Name: "Ren\u00e9 Mart\u00edn", Role: "Actor", ProviderID: "8691"})

	list, err := persons.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "8691", list[0].ProviderIDs["tmdb"])
}

// A person stored without an id (credited before the provider gave one) is
// claimed by the first credit carrying an id under that name; a later
// homonym with ANOTHER id is a new person, not the one already claimed.
func TestEnrichItem_NameFallbackOnlyForPersonWithoutID(t *testing.T) {
	e, _, persons, _, miRepo, _ := newTestEnricher(&fakeProvider{name: "tmdb"}, nil, nil)
	ctx := context.Background()
	for _, id := range []string{"item-a", "item-b", "item-c"} {
		require.NoError(t, miRepo.Create(ctx, &domainlib.MediaItem{ID: shared.MediaItemID(id), Type: "movie"}))
	}

	enrichCast(t, e, "item-a", domain.PersonResult{Name: "Sam Taylor", Role: "Actor"})
	enrichCast(t, e, "item-b", domain.PersonResult{Name: "Sam Taylor", Role: "Actor", ProviderID: "101"})
	list, err := persons.List(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"Sam Taylor/101"}, personNamesAndIDs(t, list), "the id-less row is claimed, not duplicated")

	enrichCast(t, e, "item-c", domain.PersonResult{Name: "Sam Taylor", Role: "Actor", ProviderID: "202"})
	list, err = persons.List(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"Sam Taylor/101", "Sam Taylor/202"}, personNamesAndIDs(t, list))
}
