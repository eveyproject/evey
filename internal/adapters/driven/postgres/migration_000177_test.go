package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// migrationDB is an empty Postgres of its own: this test walks the schema
// down and up, which the shared database of the package must never see.
func migrationDB(t *testing.T) *sql.DB {
	t.Helper()
	testhelpers.SkipIfNoDocker(t)
	ctx := context.Background()
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "postgres:16-alpine",
			ExposedPorts: []string{"5432/tcp"},
			Cmd:          []string{"-c", "fsync=off", "-c", "synchronous_commit=off", "-c", "full_page_writes=off"},
			Env:          map[string]string{"POSTGRES_USER": "evey_test", "POSTGRES_PASSWORD": "evey_test", "POSTGRES_DB": "evey_test"},
			WaitingFor: wait.ForAll(
				wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
				wait.ForListeningPort("5432/tcp"),
			).WithStartupTimeoutDefault(90 * time.Second),
		},
		Started: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Terminate(ctx) })
	host, err := c.Host(ctx)
	require.NoError(t, err)
	port, err := c.MappedPort(ctx, "5432")
	require.NoError(t, err)
	db, err := sql.Open("postgres", fmt.Sprintf("host=%s port=%s user=evey_test password=evey_test dbname=evey_test sslmode=disable", host, port.Port()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.Eventually(t, func() bool { return db.Ping() == nil }, 15*time.Second, 200*time.Millisecond)
	return db
}

// Rows written before 000177 (no file size, mtime or source fingerprint) are
// stamped with the file each one points at: after the migration nothing is
// listed for regeneration or re-detection and everything is served as before,
// while a real replacement still invalidates them.
func TestMigration000177_ExistingRowsAreStampedNotRegenerated(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	db := migrationDB(t)
	m := newMigrator(t, db)
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatal(err)
	}
	ctx := context.Background()
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	tpRepo := postgres.NewTrickplayRepository(db)
	segRepo := postgres.NewMediaSegmentRepository(db)

	lib := newTestLibrary("mig177")
	require.NoError(t, libRepo.Create(ctx, lib))
	season := newTestMediaItem("mig177-season", lib.ID)
	season.Type = library.Season
	require.NoError(t, itemRepo.Create(ctx, season))
	mod := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	ep := newTestMediaItem("mig177-ep", lib.ID)
	ep.Type, ep.ParentID = library.Episode, string(season.ID)
	ep.File = &library.MediaFile{Path: "/media/ep.mkv", Size: 100, ModTime: mod, Fingerprint: fingerprintOf(100, mod)}
	require.NoError(t, itemRepo.Create(ctx, ep))
	require.NoError(t, tpRepo.Create(ctx, &library.TrickplayManifest{ID: "mig177-tp", MediaItemID: ep.ID, ImagePath: "/t", AlgoVersion: 1, FileSize: 100, ModTime: mod, CreatedAt: time.Now()}))
	_, err := segRepo.UpsertUnlessManual(ctx, &library.MediaSegment{MediaItemID: ep.ID, Type: "intro", StartMs: 30_000, EndMs: 90_000, Confidence: 1, Source: "mkv_chapter"})
	require.NoError(t, err)
	require.NoError(t, segRepo.Upsert(ctx, &library.MediaSegment{MediaItemID: ep.ID, Type: "end_credits", StartMs: 2_000_000, EndMs: 2_100_000, Confidence: 1, Source: "manual"}))

	// Back to the schema the server runs today: the columns go, the rows stay.
	require.NoError(t, m.Migrate(176))
	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'trickplay_manifests' AND column_name = 'file_size'`).Scan(&n))
	require.Zero(t, n, "premise: 176 has no file_size")

	// The first deployment.
	require.NoError(t, m.Migrate(177))

	var size sql.NullInt64
	var mtime sql.NullTime
	require.NoError(t, db.QueryRow(`SELECT file_size, mod_time FROM trickplay_manifests WHERE media_item_id = $1`, string(ep.ID)).Scan(&size, &mtime))
	assert.Equal(t, int64(100), size.Int64)
	assert.True(t, mtime.Valid && mtime.Time.Equal(mod), "mod_time = %v", mtime)
	rows, err := db.Query(`SELECT source, source_fingerprint FROM media_segments WHERE media_item_id = $1`, string(ep.ID))
	require.NoError(t, err)
	defer rows.Close()
	stamps := map[string]sql.NullString{}
	for rows.Next() {
		var src string
		var fp sql.NullString
		require.NoError(t, rows.Scan(&src, &fp))
		stamps[src] = fp
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, ep.File.Fingerprint, stamps["mkv_chapter"].String, "a detected segment is stamped with its file")
	assert.False(t, stamps["manual"].Valid, "an operator mark stays unstamped")

	missing, err := tpRepo.ListMissingMediaItemIDs(ctx, 0)
	require.NoError(t, err)
	assert.Empty(t, missing, "the migration lists sprites for regeneration")
	pending, err := segRepo.ListSeasonsPendingIntroDetection(ctx, 5, 2, 100, nil)
	require.NoError(t, err)
	assert.NotContains(t, pending, string(season.ID), "the migration reopens intro detection")
	_, err = tpRepo.GetByMediaItemID(ctx, ep.ID)
	require.NoError(t, err, "the sprites are no longer served after the migration")
	segs, err := segRepo.GetByMediaItemID(ctx, ep.ID)
	require.NoError(t, err)
	assert.Len(t, segs, 2)

	// A real replacement still invalidates.
	replaceFile(t, itemRepo, ep, 100, mod.Add(time.Hour))
	missing, err = tpRepo.ListMissingMediaItemIDs(ctx, 0)
	require.NoError(t, err)
	assert.Equal(t, []string{string(ep.ID)}, missing)
	pending, err = segRepo.ListSeasonsPendingIntroDetection(ctx, 5, 2, 100, nil)
	require.NoError(t, err)
	assert.Contains(t, pending, string(season.ID))
	_, err = tpRepo.GetByMediaItemID(ctx, ep.ID)
	require.ErrorIs(t, err, shared.ErrNotFound)
}

// 000177 joins on media_item_id alone, which is exact: media_files is keyed by
// media_item_id, so an item has one file at most and a second one is refused.
// Two versions of one film are two items, two files: each row stays on its own.
func TestMigration000177_TwoVersionsOfAFilmKeepTheirOwnFile(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	db := migrationDB(t)
	m := newMigrator(t, db)
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatal(err)
	}
	ctx := context.Background()
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	tpRepo := postgres.NewTrickplayRepository(db)
	segRepo := postgres.NewMediaSegmentRepository(db)
	lib := newTestLibrary("mig177v")
	require.NoError(t, libRepo.Create(ctx, lib))

	modA := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	modB := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	versions := []struct {
		id   string
		path string
		size int64
		mod  time.Time
	}{
		{"mig177-1080p", "/media/Film (2001) - 1080P.mkv", 8_000, modA},
		{"mig177-2160p", "/media/Film (2001) - 2160P.mkv", 40_000, modB},
	}
	for _, v := range versions {
		it := newTestMediaItem(v.id, lib.ID)
		it.Title = "Film"
		it.File = &library.MediaFile{Path: v.path, Size: v.size, ModTime: v.mod, Fingerprint: fingerprintOf(v.size, v.mod)}
		require.NoError(t, itemRepo.Create(ctx, it))
		require.NoError(t, tpRepo.Create(ctx, &library.TrickplayManifest{ID: v.id + "-tp", MediaItemID: it.ID, ImagePath: "/t/" + v.id,
			AlgoVersion: 1, FileSize: v.size, ModTime: v.mod, CreatedAt: time.Now()}))
		_, err := segRepo.UpsertUnlessManual(ctx, &library.MediaSegment{MediaItemID: it.ID, Type: "intro", StartMs: 30_000, EndMs: 90_000, Confidence: 1, Source: "mkv_chapter"})
		require.NoError(t, err)
	}
	// The premise: a second file for one item does not exist in this schema.
	_, err := db.Exec(`INSERT INTO media_files (media_item_id, file_path, file_size) VALUES ($1, '/media/other.mkv', 1)`, versions[0].id)
	require.Error(t, err, "media_files accepted a second file for one item")

	require.NoError(t, m.Migrate(176))
	require.NoError(t, m.Migrate(177))

	for _, v := range versions {
		var size sql.NullInt64
		var mtime sql.NullTime
		require.NoError(t, db.QueryRow(`SELECT file_size, mod_time FROM trickplay_manifests WHERE media_item_id = $1`, v.id).Scan(&size, &mtime))
		assert.Equal(t, v.size, size.Int64, "%s: sprites stamped with another file", v.id)
		assert.True(t, mtime.Valid && mtime.Time.Equal(v.mod), "%s: mod_time = %v", v.id, mtime)
		var fp sql.NullString
		require.NoError(t, db.QueryRow(`SELECT source_fingerprint FROM media_segments WHERE media_item_id = $1`, v.id).Scan(&fp))
		assert.Equal(t, fingerprintOf(v.size, v.mod), fp.String, "%s: segment stamped with another file", v.id)
	}
	missing, err := tpRepo.ListMissingMediaItemIDs(ctx, 0)
	require.NoError(t, err)
	assert.Empty(t, missing, "a version's sprites were listed for regeneration")
}
