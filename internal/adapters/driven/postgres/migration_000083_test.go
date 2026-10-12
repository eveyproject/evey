package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigration000083_DedupeSeriesSeasons verifies the data-integrity
// semantics of migration 000083: duplicate series/seasons are collapsed
// to the oldest row (by added_at), seasons of losing series are
// reparented to the winning series, episodes of losing seasons are
// reparented to the winning season, and nothing outside the duplicated
// (library_id, scan_title) pair is touched.
//
// Strategy: StartPostgres brings the schema to HEAD (migration already ran
// once on empty data - a no-op). We then seed the pre-dedup mess and
// re-execute the real migration SQL so the test always exercises the
// exact statements shipped to prod.
func TestMigration000083_DedupeSeriesSeasons(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	ctx := context.Background()
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)

	// Seed two libraries. Type doesn't affect dedup logic; reuse the
	// newTestLibrary helper (Movie type) to avoid duplicating boilerplate.
	require.NoError(t, libRepo.Create(ctx, newTestLibrary("lib-083")))
	require.NoError(t, libRepo.Create(ctx, newTestLibrary("lib-083b")))

	// Deterministic added_at timestamps so "oldest wins" is unambiguous.
	at2020 := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	at2021 := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	at2019 := time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC)

	// ── lib-083: two series with the same scan_title ─────────────────────────
	//
	//   series-old (2020) → WINNER
	//     season-old S1 (2020-02) → WINNER season
	//       episode-old
	//
	//   series-new (2021) → LOSER (will be deleted)
	//     season-new S1 (2021-02) → LOSER season
	//       episode-new           → must be reparented to season-old S1

	idxS1a := 1
	require.NoError(t, itemRepo.Create(ctx, &library.MediaItem{
		ID: "mi-series-old", LibraryID: "lib-083",
		Type: library.Series, Title: "Some Series", ScanTitle: "Some Series",
		SortTitle: "some series",
		AddedAt:   at2020, UpdatedAt: at2020,
	}))
	require.NoError(t, itemRepo.Create(ctx, &library.MediaItem{
		ID: "mi-series-new", LibraryID: "lib-083",
		Type: library.Series, Title: "Some Series", ScanTitle: "Some Series",
		SortTitle: "some series",
		AddedAt:   at2021, UpdatedAt: at2021,
	}))

	seasonOldAt := time.Date(2020, 2, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, itemRepo.Create(ctx, &library.MediaItem{
		ID: "mi-season-old", LibraryID: "lib-083",
		ParentID: "mi-series-old",
		Type:     library.Season, Title: "Season 1", ScanTitle: "Season 1",
		SortTitle: "season 01", IndexNumber: &idxS1a,
		AddedAt: seasonOldAt, UpdatedAt: seasonOldAt,
	}))

	idxS1b := 1
	seasonNewAt := time.Date(2021, 2, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, itemRepo.Create(ctx, &library.MediaItem{
		ID: "mi-season-new", LibraryID: "lib-083",
		ParentID: "mi-series-new",
		Type:     library.Season, Title: "Season 1", ScanTitle: "Season 1",
		SortTitle: "season 01", IndexNumber: &idxS1b,
		AddedAt: seasonNewAt, UpdatedAt: seasonNewAt,
	}))

	episodeOldAt := time.Date(2020, 3, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, itemRepo.Create(ctx, &library.MediaItem{
		ID: "mi-episode-old", LibraryID: "lib-083",
		ParentID: "mi-season-old",
		Type:     library.Episode, Title: "Pilot", ScanTitle: "Pilot",
		SortTitle: "pilot",
		AddedAt:   episodeOldAt, UpdatedAt: episodeOldAt,
	}))

	episodeNewAt := time.Date(2021, 3, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, itemRepo.Create(ctx, &library.MediaItem{
		ID: "mi-episode-new", LibraryID: "lib-083",
		ParentID: "mi-season-new",
		Type:     library.Episode, Title: "The First Cook", ScanTitle: "The First Cook",
		SortTitle: "the first cook",
		AddedAt:   episodeNewAt, UpdatedAt: episodeNewAt,
	}))

	// ── lib-083b: negative guard - same scan_title, different library ─────────
	// Dedup is scoped to (library_id, scan_title), so this series/season/episode
	// must survive completely unchanged.

	idxKeep := 1
	require.NoError(t, itemRepo.Create(ctx, &library.MediaItem{
		ID: "mi-series-keep", LibraryID: "lib-083b",
		Type: library.Series, Title: "Some Series", ScanTitle: "Some Series",
		SortTitle: "some series",
		AddedAt:   at2019, UpdatedAt: at2019,
	}))

	seasonKeepAt := time.Date(2019, 2, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, itemRepo.Create(ctx, &library.MediaItem{
		ID: "mi-season-keep", LibraryID: "lib-083b",
		ParentID: "mi-series-keep",
		Type:     library.Season, Title: "Season 1", ScanTitle: "Season 1",
		SortTitle: "season 01", IndexNumber: &idxKeep,
		AddedAt: seasonKeepAt, UpdatedAt: seasonKeepAt,
	}))

	episodeKeepAt := time.Date(2019, 3, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, itemRepo.Create(ctx, &library.MediaItem{
		ID: "mi-episode-keep", LibraryID: "lib-083b",
		ParentID: "mi-season-keep",
		Type:     library.Episode, Title: "Pilot", ScanTitle: "Pilot",
		SortTitle: "pilot",
		AddedAt:   episodeKeepAt, UpdatedAt: episodeKeepAt,
	}))

	// ── Re-execute migration 000083 against the seeded dupes ─────────────────
	// Reading the real file keeps the test in sync with the shipped migration.
	// The file wraps its logic in BEGIN/COMMIT with TEMP TABLE ON COMMIT DROP,
	// so it must run as a single Exec (simple query protocol, no $n params).
	sqlBytes, err := os.ReadFile("migrations/000083_dedupe_series_seasons.up.sql")
	require.NoError(t, err, "migration SQL file must be readable")

	_, err = db.ExecContext(ctx, string(sqlBytes))
	require.NoError(t, err, "migration 000083 must execute cleanly on seeded dupes")

	// ── Assert dedup outcomes ─────────────────────────────────────────────────

	// 1. Exactly one series survives in lib-083, and it must be the oldest.
	var seriesCount int
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT count(*) FROM media_items WHERE type='series' AND library_id='lib-083'`,
	).Scan(&seriesCount))
	assert.Equal(t, 1, seriesCount, "exactly one series must survive dedup")

	var survivingSeriesID string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT id FROM media_items WHERE type='series' AND library_id='lib-083'`,
	).Scan(&survivingSeriesID))
	assert.Equal(t, "mi-series-old", survivingSeriesID, "oldest series (added_at 2020) must win")

	// 2. Exactly one season under the surviving series (oldest wins).
	var seasonCount int
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT count(*) FROM media_items WHERE type='season' AND parent_id='mi-series-old'`,
	).Scan(&seasonCount))
	assert.Equal(t, 1, seasonCount, "exactly one season must survive under winning series")

	var survivingSeasonID string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT id FROM media_items WHERE type='season' AND parent_id='mi-series-old'`,
	).Scan(&survivingSeasonID))
	assert.Equal(t, "mi-season-old", survivingSeasonID, "oldest season (added_at 2020-02) must win")

	// 3. Both episodes survive under the winning season - no data lost.
	// episode-new must have been reparented from mi-season-new → mi-season-old.
	var episodeCount int
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT count(*) FROM media_items WHERE type='episode' AND parent_id='mi-season-old'`,
	).Scan(&episodeCount))
	assert.Equal(t, 2, episodeCount, "both episodes must survive reparented to the winning season")

	epRows, err := db.QueryContext(ctx,
		`SELECT id FROM media_items WHERE type='episode' AND parent_id='mi-season-old' ORDER BY id`)
	require.NoError(t, err)
	defer epRows.Close()
	var episodeIDs []string
	for epRows.Next() {
		var id string
		require.NoError(t, epRows.Scan(&id))
		episodeIDs = append(episodeIDs, id)
	}
	require.NoError(t, epRows.Err())
	assert.ElementsMatch(t, []string{"mi-episode-old", "mi-episode-new"}, episodeIDs,
		"episode-new must have been reparented to the winning season; neither episode may be lost",
	)

	// 4. Negative guard: lib-083b untouched - series, season, and episode all survive.
	var keepSeriesCount int
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT count(*) FROM media_items WHERE library_id='lib-083b' AND type='series'`,
	).Scan(&keepSeriesCount))
	assert.Equal(t, 1, keepSeriesCount, "guard series in lib-083b must not be touched")

	var keepSeasonCount int
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT count(*) FROM media_items WHERE library_id='lib-083b' AND type='season'`,
	).Scan(&keepSeasonCount))
	assert.Equal(t, 1, keepSeasonCount, "guard season in lib-083b must not be touched")

	var keepEpisodeCount int
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT count(*) FROM media_items WHERE library_id='lib-083b' AND type='episode'`,
	).Scan(&keepEpisodeCount))
	assert.Equal(t, 1, keepEpisodeCount, "guard episode in lib-083b must not be touched")

	// Verify the guard's parent_id chain is fully intact.
	var guardSeasonParent string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT parent_id FROM media_items WHERE id='mi-season-keep'`,
	).Scan(&guardSeasonParent))
	assert.Equal(t, "mi-series-keep", guardSeasonParent, "guard season parent_id must be unchanged")

	var guardEpisodeParent string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT parent_id FROM media_items WHERE id='mi-episode-keep'`,
	).Scan(&guardEpisodeParent))
	assert.Equal(t, "mi-season-keep", guardEpisodeParent, "guard episode parent_id must be unchanged")
}
