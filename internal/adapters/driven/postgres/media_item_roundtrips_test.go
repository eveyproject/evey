package postgres_test

// A scan writes one MediaItem per file, so every round trip MediaItemRepository
// issues per write is multiplied by the size of the library. Create and Update
// used to chain BEGIN, the media_items write, a fingerprint clear, one upsert or
// delete per sub-table (file, catalog, classification), a trailer delete, a
// trailer insert and COMMIT: 8 to 10 trips for one file, 100 000 on a scan of
// 10 000. These tests pin the count AND prove the state left in the database is
// the one the chained statements produced.

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// tripLog records every statement that crosses to the server, BEGIN and COMMIT
// included, because they are round trips too.
type tripLog struct {
	mu    sync.Mutex
	stmts []string
}

func (l *tripLog) add(s string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.stmts = append(l.stmts, s)
}

func (l *tripLog) reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.stmts = nil
}

func (l *tripLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.stmts...)
}

// countingConn wraps the driver connection so the repository, which takes a
// *sql.DB, can be measured without changing production code. database/sql
// prefers the Context variants when the wrapped connection has them, so those
// are the ones to intercept.
type countingConn struct {
	driver.Conn
	log *tripLog
}

func (c *countingConn) ExecContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Result, error) {
	c.log.add(q)
	return c.Conn.(driver.ExecerContext).ExecContext(ctx, q, args)
}

func (c *countingConn) QueryContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	c.log.add(q)
	return c.Conn.(driver.QueryerContext).QueryContext(ctx, q, args)
}

func (c *countingConn) PrepareContext(ctx context.Context, q string) (driver.Stmt, error) {
	c.log.add(q)
	return c.Conn.(driver.ConnPrepareContext).PrepareContext(ctx, q)
}

func (c *countingConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	c.log.add("BEGIN")
	tx, err := c.Conn.(driver.ConnBeginTx).BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &countingTx{Tx: tx, log: c.log}, nil
}

type countingTx struct {
	driver.Tx
	log *tripLog
}

func (t *countingTx) Commit() error {
	t.log.add("COMMIT")
	return t.Tx.Commit()
}

func (t *countingTx) Rollback() error {
	t.log.add("ROLLBACK")
	return t.Tx.Rollback()
}

type countingConnector struct {
	conn driver.Conn
	log  *tripLog
	drv  driver.Driver
}

func (c *countingConnector) Connect(context.Context) (driver.Conn, error) {
	return &countingConn{Conn: c.conn, log: c.log}, nil
}
func (c *countingConnector) Driver() driver.Driver { return c.drv }

// countingDB returns a single-connection *sql.DB that rides on one connection
// borrowed from the shared test database, plus the log of what crossed it.
func countingDB(t *testing.T, shared *sql.DB) (*sql.DB, *tripLog) {
	t.Helper()
	ctx := context.Background()
	held, err := shared.Conn(ctx)
	require.NoError(t, err)
	log := &tripLog{}
	var wrapped *countingConnector
	require.NoError(t, held.Raw(func(dc any) error {
		wrapped = &countingConnector{conn: dc.(driver.Conn), log: log, drv: shared.Driver()}
		return nil
	}))
	db := sql.OpenDB(wrapped)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		// The pool must not close the borrowed connection: hand it back instead.
		_ = held.Close()
	})
	return db, log
}

func fullItem(id string, libID shared.LibraryID, fp string) *library.MediaItem {
	year := 2021
	premiere := time.Date(2021, 3, 4, 0, 0, 0, 0, time.UTC)
	rating := 7.8
	idx := 3
	it := newTestMediaItem(id, libID)
	it.IndexNumber = &idx
	it.OriginalTitle = "Original " + id
	it.Overview = "Overview " + id
	it.Tagline = "Tagline " + id
	it.File = &library.MediaFile{
		Path: "/media/" + id + ".mkv", Size: 1234, Fingerprint: fp,
		ModTime: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC), Duration: dur(90 * time.Minute),
		Bitrate: 5000, ContainerFormat: "matroska", StreamProbeAttempts: 2,
		InterleaveBackstepMs: 40, StreamProbeVersion: 9,
	}
	it.Catalog = &library.MediaCatalog{
		PremiereDate: &premiere, Year: &year,
		ProviderIDs: map[string]string{"tmdb": "42", "imdb": "tt42"},
		Genres:      []string{"Drama", "Action"}, Studios: []string{"S1", "S2"}, Tags: []string{"t1"},
	}
	it.Classification = &library.MediaClassification{
		CommunityRating: &rating, OfficialRating: "PG-13",
		CertificationsByCountry: map[string]string{"US": "PG-13", "FR": "12"},
		ProviderRatings:         map[string]float64{"tmdb": 7.5, "imdb": 8.1},
	}
	it.Trailers = []library.Trailer{
		{Site: "YouTube", Key: "k0", Language: "en", Official: true, Type: "Trailer"},
		{Site: "YouTube", Key: "k1", Language: "fr", Type: "Teaser", UseSubtitles: true, SubtitleLanguage: "fr"},
		{Site: "Vimeo", Key: "k2", Language: "de", Type: "Clip"},
	}
	return it
}

func rowCount(t *testing.T, db *sql.DB, table, col, id string) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM `+table+` WHERE `+col+` = $1`, id).Scan(&n))
	return n
}

// The count is the finding: before the change it reads 8 (typical) to 9 (full)
// for Create and 9 to 10 for Update.
func TestMediaItemRepo_WriteRoundTrips(t *testing.T) {
	shared_, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, postgres.NewLibraryRepository(shared_).Create(ctx, newTestLibrary("rtlib")))

	db, log := countingDB(t, shared_)
	repo := postgres.NewMediaItemRepository(db)

	typical := func(id string) *library.MediaItem {
		it := newTestMediaItem(id, "rtlib")
		it.File = &library.MediaFile{Path: "/media/" + id + ".mkv", Size: 10, Fingerprint: "fp-" + id, ModTime: time.Unix(1700000000, 0).UTC()}
		return it
	}

	const maxTrips = 1
	cases := []struct {
		name  string
		build func(id string) *library.MediaItem
	}{
		{"typical", typical},
		{"full", func(id string) *library.MediaItem { return fullItem(id, "rtlib", "fp-"+id) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			item := c.build("rt-" + c.name)

			log.reset()
			require.NoError(t, repo.Create(ctx, item))
			create := log.snapshot()
			t.Logf("Create(%s): %d round trips", c.name, len(create))
			for _, s := range create {
				t.Logf("  %.70s", s)
			}
			assert.LessOrEqual(t, len(create), maxTrips, "Create(%s) must stay within %d round trip(s)", c.name, maxTrips)

			item.Title = "Renamed " + c.name
			log.reset()
			require.NoError(t, repo.Update(ctx, item))
			update := log.snapshot()
			t.Logf("Update(%s): %d round trips", c.name, len(update))
			for _, s := range update {
				t.Logf("  %.70s", s)
			}
			assert.LessOrEqual(t, len(update), maxTrips, "Update(%s) must stay within %d round trip(s)", c.name, maxTrips)
		})
	}
}

// Every state the chained statements could leave must be left by the batched
// one. The scenario walks create, change, clear every optional part, re-add
// them, a missing item, and a rolled-back ambient transaction; the expectations
// are written out so they hold for both implementations.
func TestMediaItemRepo_WriteStateEquivalence(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	repo := postgres.NewMediaItemRepository(db)
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, newTestLibrary("eqlib")))

	item := fullItem("eq1", "eqlib", "fp-eq1")
	require.NoError(t, repo.Create(ctx, item))

	got, err := repo.GetByID(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "Movie eq1", got.ScanTitle, "scan_title falls back to the title on create")
	assert.Equal(t, item.File, got.File)
	assert.Equal(t, item.Catalog, got.Catalog)
	assert.Equal(t, item.Classification, got.Classification)
	assert.Equal(t, item.Trailers, got.Trailers)
	assert.Equal(t, item.IndexNumber, got.IndexNumber)
	assert.Equal(t, 3, rowCount(t, db, "media_trailers", "media_item_id", "eq1"))

	// Change everything: fewer trailers, new ids reusing positions 0 and 1.
	item.Title = "Changed"
	item.ScanTitle = ""
	item.File.Size = 999
	item.File.Duration = dur(time.Minute)
	item.Catalog.Genres = []string{"Comedy"}
	item.Catalog.Tags = nil
	item.Classification.OfficialRating = "R"
	item.Trailers = []library.Trailer{{Site: "YouTube", Key: "new0", Language: "en", Type: "Trailer"}}
	require.NoError(t, repo.Update(ctx, item))
	got, err = repo.GetByID(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "Changed", got.Title)
	assert.Equal(t, "Movie eq1", got.ScanTitle, "an empty scan_title keeps the stored one")
	assert.EqualValues(t, 999, got.File.Size)
	assert.Equal(t, []string{"Comedy"}, got.Catalog.Genres)
	assert.Empty(t, got.Catalog.Tags)
	assert.Equal(t, "R", got.Classification.OfficialRating)
	require.Len(t, got.Trailers, 1)
	assert.Equal(t, "new0", got.Trailers[0].Key)

	// Clear every optional part: nil means erase, and no row may linger.
	item.File, item.Catalog, item.Classification, item.Trailers = nil, nil, nil, nil
	require.NoError(t, repo.Update(ctx, item))
	got, err = repo.GetByID(ctx, item.ID)
	require.NoError(t, err)
	assert.Nil(t, got.File)
	assert.Nil(t, got.Catalog)
	assert.Nil(t, got.Classification)
	assert.Empty(t, got.Trailers)
	for _, tc := range []struct{ table, col string }{
		{"media_files", "media_item_id"}, {"media_catalog", "media_item_id"},
		{"media_classifications", "media_item_id"}, {"media_trailers", "media_item_id"},
	} {
		assert.Zero(t, rowCount(t, db, tc.table, tc.col, "eq1"), tc.table)
	}

	// Parts that are present but empty also mean erase (the hasData rule).
	item.File = &library.MediaFile{}
	item.Catalog = &library.MediaCatalog{Genres: []string{}}
	item.Classification = &library.MediaClassification{}
	require.NoError(t, repo.Update(ctx, item))
	assert.Zero(t, rowCount(t, db, "media_files", "media_item_id", "eq1"))
	assert.Zero(t, rowCount(t, db, "media_catalog", "media_item_id", "eq1"))
	assert.Zero(t, rowCount(t, db, "media_classifications", "media_item_id", "eq1"))

	// Re-add on an item that owns none: upsert must insert.
	item2 := fullItem("eq1", "eqlib", "fp-eq1-b")
	item2.ID = item.ID
	require.NoError(t, repo.Update(ctx, item2))
	got, err = repo.GetByID(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, item2.File, got.File)
	assert.Equal(t, item2.Catalog, got.Catalog)
	assert.Equal(t, item2.Classification, got.Classification)
	assert.Equal(t, item2.Trailers, got.Trailers)

	// An item that does not exist: ErrNotFound and nothing written for it.
	ghost := fullItem("ghost", "eqlib", "fp-ghost")
	err = repo.Update(ctx, ghost)
	require.ErrorIs(t, err, shared.ErrNotFound)
	for _, tc := range []string{"media_files", "media_catalog", "media_classifications", "media_trailers"} {
		assert.Zero(t, rowCount(t, db, tc, "media_item_id", "ghost"), tc)
	}
	assert.Equal(t, 1, rowCount(t, db, "media_files", "file_fingerprint", "fp-eq1-b"))
	assert.Zero(t, rowCount(t, db, "media_files", "file_fingerprint", "fp-ghost"))

	// A failure after the write rolls the whole ambient unit back.
	unit := postgres.NewTxUnit(db)
	item3 := fullItem("eq3", "eqlib", "fp-eq3")
	boom := assert.AnError
	err = unit.Do(ctx, func(ctx context.Context) error {
		if createErr := repo.Create(ctx, item3); createErr != nil {
			return createErr
		}
		return boom
	})
	require.ErrorIs(t, err, boom)
	assert.Zero(t, rowCount(t, db, "media_items", "id", "eq3"))
	assert.Zero(t, rowCount(t, db, "media_files", "media_item_id", "eq3"))
	assert.Zero(t, rowCount(t, db, "media_trailers", "media_item_id", "eq3"))

	// ...and a committed one keeps it.
	require.NoError(t, unit.Do(ctx, func(ctx context.Context) error { return repo.Create(ctx, item3) }))
	assert.Equal(t, 1, rowCount(t, db, "media_files", "media_item_id", "eq3"))
	assert.Equal(t, 3, rowCount(t, db, "media_trailers", "media_item_id", "eq3"))

	// A duplicate create fails as a whole: no half-written sub-tables.
	dup := fullItem("eq3", "eqlib", "fp-eq3-dup")
	require.Error(t, repo.Create(ctx, dup))
	var fp string
	require.NoError(t, db.QueryRow(`SELECT file_fingerprint FROM media_files WHERE media_item_id = 'eq3'`).Scan(&fp))
	assert.Equal(t, "fp-eq3", fp)
}

// The fingerprint hand-over must survive being part of the single statement:
// the loser is cleared before the winner's row lands, on Create and on Update.
func TestMediaItemRepo_WriteStateEquivalence_FingerprintHandOver(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	repo := postgres.NewMediaItemRepository(db)
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, newTestLibrary("fhlib")))

	a := fullItem("fh-a", "fhlib", "same")
	require.NoError(t, repo.Create(ctx, a))
	b := fullItem("fh-b", "fhlib", "same")
	require.NoError(t, repo.Create(ctx, b))
	assert.Equal(t, 1, rowCount(t, db, "media_files", "file_fingerprint", "same"))
	gotA, err := repo.GetByID(ctx, a.ID)
	require.NoError(t, err)
	assert.Empty(t, gotA.File.Fingerprint)

	// Update takes it back from b.
	require.NoError(t, repo.Update(ctx, a))
	gotA, err = repo.GetByID(ctx, a.ID)
	require.NoError(t, err)
	assert.Equal(t, "same", gotA.File.Fingerprint)
	gotB, err := repo.GetByID(ctx, b.ID)
	require.NoError(t, err)
	assert.Empty(t, gotB.File.Fingerprint)
	assert.Equal(t, "/media/fh-b.mkv", gotB.File.Path, "the loser keeps its row")
}

func dur(d time.Duration) shared.Duration {
	v, _ := shared.NewDurationFromTime(d)
	return v
}
