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

	// Series, each with a season and two episodes: one with no rating of its
	// own, as every scanned episode is, and one carrying a rating, which the
	// series' must not lower.
	n = 0
	for _, official := range officials {
		for _, certs := range certSets[:4] {
			for _, tags := range tagSets[:2] {
				series := newTestMediaItem(fmt.Sprintf("ps-%03d", n), lib.ID)
				series.Type = library.Series
				series.Catalog.Tags = tags
				series.Classification = &library.MediaClassification{OfficialRating: official, CertificationsByCountry: certs}
				require.NoError(t, itemRepo.Create(ctx, series))
				season := newTestMediaItem(fmt.Sprintf("ps-%03d-s", n), lib.ID)
				season.Type, season.ParentID = library.Season, string(series.ID)
				require.NoError(t, itemRepo.Create(ctx, season))
				ep := newTestMediaItem(fmt.Sprintf("ps-%03d-e1", n), lib.ID)
				ep.Type, ep.ParentID = library.Episode, string(season.ID)
				require.NoError(t, itemRepo.Create(ctx, ep))
				rated := newTestMediaItem(fmt.Sprintf("ps-%03d-e2", n), lib.ID)
				rated.Type, rated.ParentID = library.Episode, string(season.ID)
				rated.Classification = &library.MediaClassification{OfficialRating: officials[(n+5)%len(officials)]}
				require.NoError(t, itemRepo.Create(ctx, rated))
				n++
			}
		}
	}
	// An episode whose series has no rating, and one with no series at all.
	unrated := newTestMediaItem("ps-unrated", lib.ID)
	unrated.Type = library.Series
	require.NoError(t, itemRepo.Create(ctx, unrated))
	ep := newTestMediaItem("ps-unrated-e", lib.ID)
	ep.Type, ep.ParentID = library.Episode, string(unrated.ID)
	require.NoError(t, itemRepo.Create(ctx, ep))
	orphan := newTestMediaItem("ps-orphan-e", lib.ID)
	orphan.Type = library.Episode
	orphan.Classification = &library.MediaClassification{OfficialRating: "PG"}
	require.NoError(t, itemRepo.Create(ctx, orphan))
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

// parentalBlocks is ratings.Blocks on a scanned item, with what a season or
// an episode inherits from its series.
func parentalBlocks(rule library.ParentalRule, it *library.MediaItem) bool {
	own := ratings.Rated{}
	if it.Classification != nil {
		own.OfficialRating, own.Certifications = it.Classification.OfficialRating, it.Classification.CertificationsByCountry
	}
	if it.Catalog != nil {
		own.Tags = it.Catalog.Tags
	}
	var series *ratings.Rated
	if sp := it.SeriesParental; sp != nil {
		series = &ratings.Rated{OfficialRating: sp.OfficialRating, Certifications: sp.CertificationsByCountry, Tags: sp.Tags}
	}
	return ratings.Blocks(rule.MaxAge, rule.Region, rule.BlockedTags, own, series)
}

// listAllIDs walks every item filter selects, by id, past List's 500-row cap.
func listAllIDs(t *testing.T, ctx context.Context, queryRepo *postgres.MediaItemQueryRepoAdapter, filter library.ListMediaItemsFilter) ([]*library.MediaItem, []string) {
	t.Helper()
	var items []*library.MediaItem
	var ids []string
	filter.Limit, filter.KeysetByID = 500, true
	for {
		page, err := queryRepo.List(ctx, filter)
		require.NoError(t, err)
		if len(page) == 0 {
			return items, ids
		}
		for _, it := range page {
			items = append(items, it)
			ids = append(ids, string(it.ID))
		}
		filter.AfterID = string(page[len(page)-1].ID)
	}
}

// assertParentalParity lists the library under each rule in SQL and checks the
// result is exactly the items ratings.Blocks lets through.
func assertParentalParity(t *testing.T, ctx context.Context, queryRepo *postgres.MediaItemQueryRepoAdapter, libID shared.LibraryID) {
	t.Helper()
	all, _ := listAllIDs(t, ctx, queryRepo, library.ListMediaItemsFilter{LibraryID: &libID})
	require.Greater(t, len(all), 500, "the corpus spans more than one page")
	for _, rule := range parentalRules() {
		var want []string
		for _, it := range all {
			if !parentalBlocks(rule, it) {
				want = append(want, string(it.ID))
			}
		}
		_, gotIDs := listAllIDs(t, ctx, queryRepo, library.ListMediaItemsFilter{LibraryID: &libID, Parental: &rule})
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

	// What the inheritance is for: an episode, which carries no rating of its
	// own, is as old as its series. ps-048 is the "TP" series, ps-032 the "R".
	kid := library.ParentalRule{MaxAge: 12, Region: "FR"}
	_, seenIDs := listAllIDs(t, ctx, queryRepo, library.ListMediaItemsFilter{LibraryID: &libID, Parental: &kid})
	assert.Contains(t, seenIDs, "ps-048-e1", "an all-ages series' episode reaches a 12-year-old")
	assert.Contains(t, seenIDs, "ps-048-s", "and so does its season")
	assert.NotContains(t, seenIDs, "ps-032-e1", "an R series' episode does not")
	assert.NotContains(t, seenIDs, "ps-unrated-e", "nor one whose series has no rating")

	// Rows the migration left uncomputed, or computed under other rating
	// tables, are unknown: a capped profile sees none of them until the
	// startup refresh rewrites them, after which the parity holds again.
	_, err := db.ExecContext(ctx, `UPDATE media_classifications SET parental_rev = NULL WHERE media_item_id < 'pr-100'`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `UPDATE media_classifications SET parental_rev = 1 WHERE media_item_id >= 'pr-100'`)
	require.NoError(t, err)
	open := library.ParentalRule{MaxAge: 18, Region: "FR"}
	hidden, _ := listAllIDs(t, ctx, queryRepo, library.ListMediaItemsFilter{LibraryID: &libID, Parental: &open})
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
