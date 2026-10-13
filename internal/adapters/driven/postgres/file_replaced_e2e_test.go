package postgres_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	applib "github.com/eveyproject/evey/internal/app/library"
	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	"github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	"github.com/eveyproject/evey/internal/domain/mkv"
	mkvfakes "github.com/eveyproject/evey/internal/domain/mkv/fakes"
	"github.com/eveyproject/evey/internal/domain/shared"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
)

// nothingFoundDetector is an intro detector that finds nothing in any file.
type nothingFoundDetector struct{}

var errNothingFound = errors.New("nothing found")

func (nothingFoundDetector) ExtractTileHashes(string, int, int, int, int) ([]uint64, error) {
	return nil, errNothingFound
}
func (nothingFoundDetector) PrecisionPass(context.Context, string, float64, float64, float64) (int64, int64, error) {
	return 0, 0, errNothingFound
}
func (nothingFoundDetector) AudioFallback(context.Context, string, float64) (int64, int64, bool, error) {
	return 0, 0, false, nil
}
func (nothingFoundDetector) DetectEndCredits(context.Context, string, float64) (int64, int64, bool, error) {
	return 0, 0, false, nil
}
func (nothingFoundDetector) DetectLeadingLogoBlock(context.Context, string, float64) (int64, int64, bool, error) {
	return 0, 0, false, nil
}
func (nothingFoundDetector) DetectStudioLogo([]uint64, int) (int64, int64, bool) { return 0, 0, false }
func (nothingFoundDetector) AudioBandedFingerprint(context.Context, string, float64, float64, int) ([]float64, error) {
	return nil, errNothingFound
}
func (nothingFoundDetector) SubtitleCueStarts(context.Context, string) ([]int64, error) {
	return nil, errNothingFound
}
func (nothingFoundDetector) SceneCutTimestamps(context.Context, string, float64, float64, float64) ([]int64, error) {
	return nil, errNothingFound
}
func (nothingFoundDetector) GrayThumbnails(context.Context, string, float64, float64, float64) ([][]byte, error) {
	return nil, errNothingFound
}

// A film replaced at the same path by other content with the same size,
// codecs and duration (only the mtime tells them apart): once the scan sees
// it, nothing derived from the old file is served - sprites, detected intro,
// chapters, ffmpeg segments - while the item, its operator mark and its
// identity stay.
func TestFileReplaced_EndToEnd_NoStaleArtifactServed(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	chapterRepo := postgres.NewChapterRepository(db)
	tpRepo := postgres.NewTrickplayRepository(db)
	segRepo := postgres.NewMediaSegmentRepository(db)
	cfgRepo := postgres.NewServerConfigRepository(db)
	bus := sharedfakes.NewFakeEventBus()
	scanner := libfakes.NewFakeFileScanner()
	prober := libfakes.NewFakeStreamProber()
	reader := mkvfakes.NewFakeContainerReader()
	tpGen := libfakes.NewFakeTrickplayGenerator()
	tpDir := t.TempDir()

	const path = "/movies/film.mkv"
	dur, err := shared.NewDurationFromTime(90 * time.Minute)
	require.NoError(t, err)
	w, h := 1920, 1080
	prober.Results[path] = &library.ProbeResult{
		Streams: []*library.MediaStream{
			{StreamType: library.StreamTypeVideo, Codec: "h264", Width: &w, Height: &h, IsDefault: true},
			{StreamType: library.StreamTypeAudio, Codec: "aac", StreamIndex: 1, IsDefault: true},
		},
		Duration:  dur,
		Container: "matroska",
	}
	reader.Containers[path] = &mkv.Container{Chapters: []mkv.Chapter{
		{Title: "Opening", StartMs: 0, EndMs: 60_000},
		{Title: "Act 1", StartMs: 60_000, EndMs: 1_800_000},
	}}

	lib := &library.Library{ID: "lib-replaced", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	require.NoError(t, libRepo.Create(ctx, lib))
	svc := applib.NewCommandService(libRepo, libRepo, itemRepo, postgres.NewMediaStreamRepository(db),
		postgres.NewPersonRepository(db), postgres.NewMediaPersonRepository(db),
		scanner, prober, libfakes.NewFakeSearchIndex(), bus,
		chapterRepo, reader, tpRepo, tpGen, tpDir, cfgRepo)
	svc.StartTrickplayWorker(ctx)
	intro := applib.NewIntroDetectionService(segRepo, tpRepo, postgres.NewMediaItemQueryRepoAdapter(db), cfgRepo, nothingFoundDetector{}, tpDir)
	intro.WithEventBus(bus)
	intro.StartWorker(ctx)

	scanAt := func(size int64, mod time.Time) {
		t.Helper()
		scanner.Results["/movies"] = []library.ScannedFile{{
			Path: path, Size: size, ModTime: mod, Fingerprint: fingerprintOf(size, mod),
			Title: "Film", Type: library.MovieItem,
		}}
		_, _, scanErr := svc.ScanLibrary(ctx, string(lib.ID))
		require.NoError(t, scanErr)
	}
	waitManifestOf := func(id shared.MediaItemID, mod time.Time) *library.TrickplayManifest {
		t.Helper()
		var m *library.TrickplayManifest
		require.Eventually(t, func() bool {
			got, getErr := tpRepo.GetByMediaItemID(ctx, id)
			if getErr != nil {
				return false
			}
			m = got
			return got.ModTime.Equal(mod)
		}, 10*time.Second, 20*time.Millisecond, "sprites of the file with mtime %v", mod)
		return m
	}

	// The old file: scanned, sprites made, an intro detected in it, chapters,
	// and an operator's credits mark.
	t0 := time.Date(2026, 10, 13, 10, 0, 0, 0, time.UTC)
	scanAt(4_000_000, t0)
	item, err := itemRepo.GetByFilePath(ctx, path)
	require.NoError(t, err)
	waitManifestOf(item.ID, t0)
	stale := filepath.Join(tpDir, string(item.ID), "old-sprite.jpg")
	require.NoError(t, os.WriteFile(stale, []byte("old"), 0o644))
	_, err = segRepo.UpsertUnlessManual(ctx, &library.MediaSegment{
		MediaItemID: item.ID, Type: "intro", StartMs: 5_000, EndMs: 65_000, Confidence: 0.9,
		Source: "phash", SourceFingerprint: item.File.Fingerprint,
	})
	require.NoError(t, err)
	require.NoError(t, segRepo.Upsert(ctx, &library.MediaSegment{
		MediaItemID: item.ID, Type: "end_credits", StartMs: 5_000_000, EndMs: 5_300_000, Confidence: 1, Source: "manual",
	}))
	chapters, err := chapterRepo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, chapters, 2)
	segHashOf := func(it *library.MediaItem) string {
		return apptranscoding.DecisionFingerprint{
			MediaID: string(it.ID), SourcePath: it.File.Path, SourceFingerprint: it.File.Fingerprint,
			VideoCodec: "h264", AudioCodec: "aac",
		}.Hash()
	}
	oldHash := segHashOf(item)

	// The new file: same path, same size, same codecs and duration, no chapters.
	reader.Containers[path] = &mkv.Container{}
	t1 := t0.Add(time.Hour)
	scanAt(4_000_000, t1)

	replaced, err := itemRepo.GetByFilePath(ctx, path)
	require.NoError(t, err)
	assert.Equal(t, item.ID, replaced.ID, "a replaced file keeps its item")

	// Sprites: the old ones are never served again and get remade.
	if got, getErr := tpRepo.GetByMediaItemID(ctx, item.ID); getErr == nil {
		assert.True(t, got.ModTime.Equal(t1), "the old file's sprites are served after the replacement")
	}
	waitManifestOf(item.ID, t1)
	_, err = os.Stat(stale)
	assert.True(t, os.IsNotExist(err), "the old file's sprite files are left on disk")

	// Segments: what a detector found in the old file is gone, the operator's mark stays.
	segs, err := segRepo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	for _, s := range segs {
		assert.NotEqual(t, "phash", s.Source, "the intro detected in the old file is served")
	}
	require.NotEmpty(t, segs)
	assert.Equal(t, "manual", segs[0].Source)

	// Chapters: the new file has none, the old ones are gone.
	chapters, err = chapterRepo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	assert.Empty(t, chapters, "the old file's chapters are served")

	// ffmpeg segments: the cache key moved with the file.
	assert.NotEqual(t, oldHash, segHashOf(replaced), "the old file's ffmpeg segments are addressable")
}
