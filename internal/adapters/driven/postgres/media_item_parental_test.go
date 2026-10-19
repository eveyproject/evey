package postgres_test

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/ratings"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// parentalCorpus seeds a library with every combination of official rating,
// country certifications and tags the parental rule reads, plus an item with
// no classification at all.
func parentalCorpus(t *testing.T, ctx context.Context, itemRepo *postgres.MediaItemRepository, lib *library.Library) {
	t.Helper()
	officials := []string{"", "G", "PG", "pg-13", "R", "NR", "TP", "-12", "-16", "ACB MA 15+", "BBFC 15", "FSK 6", "bogus"}
	certSets := []map[string]string{
		nil, {"AU": "MA 15+"}, {"FR": "-10"}, {"GB": "U"}, {"DE": "18", "FR": "TP"}, {"xx": "weird"}, {"US": "NR"},
	}
	tagSets := [][]string{{}, {"Horror"}, {" violence ", "Family"}}
	n := 0
	for _, official := range officials {
		for _, certs := range certSets {
			for _, tags := range tagSets {
				it := newTestMediaItem(fmt.Sprintf("pr-%03d", n), lib.ID)
				n++
				it.Catalog.Tags = tags
				it.Classification = &library.MediaClassification{OfficialRating: official, CertificationsByCountry: certs}
				require.NoError(t, itemRepo.Create(ctx, it))
			}
		}
	}
	bare := newTestMediaItem("pr-bare", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, bare))
}

func parentalRules() []library.ParentalRule {
	var out []library.ParentalRule
	for _, region := range []string{"FR", "US", "GB", "DE", "BE"} {
		for _, age := range []int{-1, 0, 10, 12, 15, 18} {
			for _, tags := range [][]string{nil, {"HORROR", "violence"}} {
				if age < 0 && tags == nil {
					continue
				}
				out = append(out, library.ParentalRule{MaxAge: age, Region: region, BlockedTags: tags})
			}
		}
	}
	return out
}

// parentalBlocks is ratings.Blocks on a scanned item.
func parentalBlocks(rule library.ParentalRule, it *library.MediaItem) bool {
	var official string
	var certs map[string]string
	if it.Classification != nil {
		official, certs = it.Classification.OfficialRating, it.Classification.CertificationsByCountry
	}
	var tags []string
	if it.Catalog != nil {
		tags = it.Catalog.Tags
	}
	return ratings.Blocks(rule.MaxAge, rule.Region, rule.BlockedTags, official, certs, tags)
}

// assertParentalParity lists the library under each rule in SQL and checks the
// result is exactly the items ratings.Blocks lets through.
func assertParentalParity(t *testing.T, ctx context.Context, queryRepo *postgres.MediaItemQueryRepoAdapter, libID shared.LibraryID) {
	t.Helper()
	all, err := queryRepo.List(ctx, library.ListMediaItemsFilter{LibraryID: &libID, Limit: 10000})
	require.NoError(t, err)
	require.NotEmpty(t, all)
	for _, rule := range parentalRules() {
		var want []string
		for _, it := range all {
			if !parentalBlocks(rule, it) {
				want = append(want, string(it.ID))
			}
		}
		got, err := queryRepo.List(ctx, library.ListMediaItemsFilter{LibraryID: &libID, Limit: 10000, Parental: &rule})
		require.NoError(t, err)
		gotIDs := make([]string, 0, len(got))
		for _, it := range got {
			gotIDs = append(gotIDs, string(it.ID))
		}
		slices.Sort(want)
		slices.Sort(gotIDs)
		require.Equal(t, want, gotIDs, "rule %+v: SQL and ratings.Blocks disagree", rule)
	}
}

// The SQL predicate hides exactly what the Go rule hides, on every rating,
// certification and tag combination, for every region and cap.
func TestParentalRule_SQLMatchesGo(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	queryRepo := postgres.NewMediaItemQueryRepoAdapter(db)

	lib := newTestLibrary("pr-lib")
	require.NoError(t, libRepo.Create(ctx, lib))
	parentalCorpus(t, ctx, itemRepo, lib)
	libID := lib.ID
	assertParentalParity(t, ctx, queryRepo, libID)

	// Rows the migration left uncomputed, or computed under other rating
	// tables, are unknown: a capped profile sees none of them until the
	// startup refresh rewrites them, after which the parity holds again.
	_, err := db.ExecContext(ctx, `UPDATE media_classifications SET parental_rev = NULL WHERE media_item_id < 'pr-100'`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `UPDATE media_classifications SET parental_rev = 1 WHERE media_item_id >= 'pr-100'`)
	require.NoError(t, err)
	open := library.ParentalRule{MaxAge: 18, Region: "FR"}
	hidden, err := queryRepo.List(ctx, library.ListMediaItemsFilter{LibraryID: &libID, Limit: 10000, Parental: &open})
	require.NoError(t, err)
	assert.Empty(t, hidden, "a stale row must read as unknown")

	n, err := itemRepo.RefreshParentalRatings(ctx)
	require.NoError(t, err)
	var rows int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM media_classifications`).Scan(&rows))
	assert.Equal(t, rows, n, "every classification row rewritten once")
	again, err := itemRepo.RefreshParentalRatings(ctx)
	require.NoError(t, err)
	assert.Zero(t, again, "nothing left to rewrite")
	assertParentalParity(t, ctx, queryRepo, libID)
}

// A series and its seasons carry no stream: every stream hangs off the
// episodes. A facet query narrowed to series therefore finds nothing, which
// is why a shows library offers no codec, resolution or language chip (the
// clients filter those on the items' own streams, and a series has none).
func TestListStreamFacets_SeriesCarryNoStreams(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	streamRepo := postgres.NewMediaStreamRepository(db)
	queryRepo := postgres.NewMediaItemQueryRepoAdapter(db)

	lib := newTestLibrary("sf-lib")
	require.NoError(t, libRepo.Create(ctx, lib))
	series := newTestMediaItem("sf-series", lib.ID)
	series.Type = library.Series
	require.NoError(t, itemRepo.Create(ctx, series))
	season := newTestMediaItem("sf-season", lib.ID)
	season.Type, season.ParentID = library.Season, string(series.ID)
	require.NoError(t, itemRepo.Create(ctx, season))
	ep := newTestMediaItem("sf-ep", lib.ID)
	ep.Type, ep.ParentID = library.Episode, string(season.ID)
	require.NoError(t, itemRepo.Create(ctx, ep))
	require.NoError(t, streamRepo.CreateBatch(ctx, []*library.MediaStream{
		facetVideo("sf-ep-v", ep.ID, 0, "h264", 1920, 1080),
		facetLang("sf-ep-a", ep.ID, 1, library.StreamTypeAudio, "fre"),
	}))

	libID := lib.ID
	seriesType := string(library.Series)
	facets, err := queryRepo.ListStreamFacets(ctx, library.ListMediaItemsFilter{LibraryID: &libID, Type: &seriesType})
	require.NoError(t, err)
	assert.Empty(t, facets.VideoCodecs)
	assert.Empty(t, facets.AudioLanguages)

	all, err := queryRepo.ListStreamFacets(ctx, library.ListMediaItemsFilter{LibraryID: &libID})
	require.NoError(t, err)
	assert.Equal(t, []string{"h264"}, all.VideoCodecs, "the episodes do carry them")
}
