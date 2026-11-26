package catalog

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

type fakeRepo struct {
	series   []library.SeriesRef
	expected map[string][]library.ExpectedEpisode
	owned    map[string][]library.OwnedEpisode
	sagas    []library.CollectionRef
	ownedIn  map[string][]int
	parts    map[string][]library.CollectionPart
	toCheck  []library.SeriesRef
	sagasDue []library.CollectionRef
	stored   map[string]int
}

func (f *fakeRepo) SeriesToCheck(context.Context, time.Time, int) ([]library.SeriesRef, error) {
	return f.toCheck, nil
}
func (f *fakeRepo) ReplaceSeriesEpisodes(_ context.Context, id string, eps []library.ExpectedEpisode, _ time.Time) error {
	f.stored[id] = len(eps)
	return nil
}
func (f *fakeRepo) CollectionsToCheck(context.Context, time.Time, int) ([]library.CollectionRef, error) {
	return f.sagasDue, nil
}
func (f *fakeRepo) ReplaceCollectionParts(_ context.Context, id string, p []library.CollectionPart, _ time.Time) error {
	f.stored[id] = len(p)
	return nil
}
func (f *fakeRepo) ListSeries(context.Context, []shared.LibraryID) ([]library.SeriesRef, error) {
	return f.series, nil
}
func (f *fakeRepo) ExpectedEpisodes(context.Context, []string) (map[string][]library.ExpectedEpisode, error) {
	return f.expected, nil
}
func (f *fakeRepo) OwnedEpisodes(context.Context, []string) (map[string][]library.OwnedEpisode, error) {
	return f.owned, nil
}
func (f *fakeRepo) SagasOwned(context.Context, []shared.LibraryID) ([]library.CollectionRef, map[string][]int, error) {
	return f.sagas, f.ownedIn, nil
}
func (f *fakeRepo) CollectionParts(context.Context, []string) (map[string][]library.CollectionPart, error) {
	return f.parts, nil
}

type fakeSource struct {
	calls []string
	fail  map[string]bool
}

func (s *fakeSource) SeriesEpisodes(_ context.Context, id string) ([]library.ExpectedEpisode, error) {
	s.calls = append(s.calls, "tv:"+id)
	if s.fail[id] {
		return nil, errors.New("tmdb down")
	}
	return []library.ExpectedEpisode{{Season: 1, Episode: 1}}, nil
}
func (s *fakeSource) CollectionParts(_ context.Context, id int) ([]library.CollectionPart, error) {
	s.calls = append(s.calls, "collection")
	return []library.CollectionPart{{TmdbID: id}}, nil
}

func day(y int, m time.Month, d int) *time.Time {
	t := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return &t
}

var today = time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC)

func newSvc(repo *fakeRepo, src library.CatalogSource) *Service {
	s := NewService(repo, src)
	s.now = func() time.Time { return today }
	s.sleep = func(context.Context, time.Duration) bool { return true }
	return s
}

func ep(s, e int) library.OwnedEpisode { return library.OwnedEpisode{Season: s, Episode: e} }

// From the catalog: aired episodes not held; a season held in full is not
// listed, a season held not at all is whole, future and unknown dates and
// specials do not count.
func TestGaps_FromCatalog(t *testing.T) {
	repo := &fakeRepo{
		series: []library.SeriesRef{{ItemID: "got", Title: "Game of Thrones", Year: 2011, TmdbID: "1399", ImdbID: "tt0944947"}},
		expected: map[string][]library.ExpectedEpisode{"got": {
			{Season: 0, Episode: 1, AirDate: day(2010, 12, 5)}, // special: never counted
			{Season: 1, Episode: 1, AirDate: day(2011, 4, 17)},
			{Season: 1, Episode: 2, AirDate: day(2011, 4, 24)},
			{Season: 2, Episode: 1, Title: "The North Remembers", AirDate: day(2012, 4, 1)},
			{Season: 2, Episode: 2, AirDate: day(2012, 4, 8)},
			{Season: 3, Episode: 1, AirDate: day(2013, 3, 31)},
			{Season: 3, Episode: 2, AirDate: day(2013, 4, 7)},
			{Season: 4, Episode: 1, AirDate: day(2027, 1, 1)}, // not out yet
			{Season: 4, Episode: 2},                           // no date
		}},
		owned: map[string][]library.OwnedEpisode{"got": {ep(1, 1), ep(1, 2), ep(2, 2)}},
	}
	g, err := newSvc(repo, nil).Gaps(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, g.Series, 1)
	s := g.Series[0]
	assert.True(t, s.FromCatalog)
	assert.Equal(t, 1399, s.TmdbID)
	assert.Equal(t, "tt0944947", s.ImdbID)
	require.Len(t, s.Seasons, 2)
	assert.Equal(t, 2, s.Seasons[0].Season)
	assert.False(t, s.Seasons[0].Whole)
	require.Len(t, s.Seasons[0].Episodes, 1)
	assert.Equal(t, "The North Remembers", s.Seasons[0].Episodes[0].Title)
	assert.Equal(t, 3, s.Seasons[1].Season)
	assert.True(t, s.Seasons[1].Whole)
	assert.Len(t, s.Seasons[1].Episodes, 2)
}

// A series the catalog does not know: only the holes in the library's own
// numbering, and a season missing below the highest held.
func TestGaps_FromNumbering(t *testing.T) {
	repo := &fakeRepo{
		series: []library.SeriesRef{{ItemID: "x", Title: "Local Show"}},
		owned:  map[string][]library.OwnedEpisode{"x": {ep(1, 1), ep(1, 2), ep(1, 4), ep(3, 1)}},
	}
	g, err := newSvc(repo, nil).Gaps(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, g.Series, 1)
	s := g.Series[0]
	assert.False(t, s.FromCatalog)
	require.Len(t, s.Seasons, 2)
	assert.Equal(t, 1, s.Seasons[0].Season)
	assert.Equal(t, []MissingEpisode{{Episode: 3}}, s.Seasons[0].Episodes)
	assert.Equal(t, 2, s.Seasons[1].Season)
	assert.True(t, s.Seasons[1].Whole)
}

// A complete series is not listed at all.
func TestGaps_CompleteSeriesIsSilent(t *testing.T) {
	repo := &fakeRepo{
		series:   []library.SeriesRef{{ItemID: "a"}},
		expected: map[string][]library.ExpectedEpisode{"a": {{Season: 1, Episode: 1, AirDate: day(2020, 1, 1)}}},
		owned:    map[string][]library.OwnedEpisode{"a": {ep(1, 1)}},
	}
	g, err := newSvc(repo, nil).Gaps(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, g.Series)
}

// A saga started: its released films not held; one not out yet does not count.
func TestGaps_Sagas(t *testing.T) {
	repo := &fakeRepo{
		sagas:   []library.CollectionRef{{ID: "c1", Name: "Rocky", TmdbID: 1575}},
		ownedIn: map[string][]int{"c1": {1366, 1367}},
		parts: map[string][]library.CollectionPart{"c1": {
			{TmdbID: 1366, Title: "Rocky", ReleaseDate: day(1976, 11, 21)},
			{TmdbID: 1367, Title: "Rocky II", ReleaseDate: day(1979, 6, 15)},
			{TmdbID: 1371, Title: "Rocky III", ReleaseDate: day(1982, 5, 28)},
			{TmdbID: 9999, Title: "Rocky VII", ReleaseDate: day(2028, 1, 1)},
		}},
	}
	g, err := newSvc(repo, nil).Gaps(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, g.Sagas, 1)
	assert.Equal(t, 2, g.Sagas[0].Owned)
	assert.Equal(t, 1575, g.Sagas[0].TmdbID)
	require.Len(t, g.Sagas[0].Missing, 1)
	assert.Equal(t, "Rocky III", g.Sagas[0].Missing[0].Title)
}

// The refresher asks only what is due, stores each answer, and a failure
// stores nothing (asked again next pass).
func TestRefreshOnce_StoresWhatIsDue(t *testing.T) {
	repo := &fakeRepo{
		toCheck:  []library.SeriesRef{{ItemID: "a", TmdbID: "1"}, {ItemID: "b", TmdbID: "2"}},
		sagasDue: []library.CollectionRef{{ID: "c1", TmdbID: 10}},
		stored:   map[string]int{},
	}
	src := &fakeSource{fail: map[string]bool{"2": true}}
	newSvc(repo, src).RefreshOnce(context.Background())
	assert.Equal(t, []string{"tv:1", "tv:2", "collection"}, src.calls)
	assert.Equal(t, map[string]int{"a": 1, "c1": 1}, repo.stored)
}

// Without a source (no TMDB key) the refresher does nothing.
func TestRun_WithoutSourceDoesNothing(t *testing.T) {
	newSvc(&fakeRepo{}, nil).Run(context.Background())
}
