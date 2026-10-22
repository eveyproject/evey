package library

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	"github.com/eveyproject/evey/internal/domain/shared"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
)

// settleGen counts its calls and runs during, when set, while it "decodes".
type settleGen struct {
	*libfakes.FakeTrickplayGenerator
	calls  atomic.Int32
	during func()
}

func (g *settleGen) Generate(ctx context.Context, filePath, outputDir string, interval, width, tileWidth, tileHeight, maxWorkers int, d int64) (*library.TrickplayResult, error) {
	g.calls.Add(1)
	if g.during != nil {
		g.during()
	}
	return g.FakeTrickplayGenerator.Generate(ctx, filePath, outputDir, interval, width, tileWidth, tileHeight, maxWorkers, d)
}

type settleFixture struct {
	svc   *TrickplayService
	gen   *settleGen
	repo  *libfakes.FakeTrickplayRepository
	media *libfakes.FakeMediaItemCommandRepo
	path  string
	mod   time.Time
}

// newSettleFixture is a 1000-byte file last written long ago, known to the
// library as it is, with a 10-minute duration; the worker runs, so a
// postponed generation comes back after retry.
func newSettleFixture(t *testing.T, probe, retry time.Duration) *settleFixture {
	t.Helper()
	path := filepath.Join(t.TempDir(), "film.mkv")
	require.NoError(t, os.WriteFile(path, make([]byte, 1000), 0o644))
	mod := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	require.NoError(t, os.Chtimes(path, mod, mod))
	dur, err := shared.NewDurationFromTime(10 * time.Minute)
	require.NoError(t, err)

	media := libfakes.NewFakeMediaItemCommandRepo()
	require.NoError(t, media.Create(context.Background(), &library.MediaItem{
		ID: "m1", Type: library.MovieItem, Title: "Film",
		File: &library.MediaFile{Path: path, Size: 1000, ModTime: mod, Duration: dur},
	}))
	repo := libfakes.NewFakeTrickplayRepository()
	gen := &settleGen{FakeTrickplayGenerator: libfakes.NewFakeTrickplayGenerator()}
	// Thumbnails every 7 s to the end: a complete file.
	frames := make([]int64, 0, 86)
	for ms := int64(0); ms < dur.Milliseconds(); ms += 7000 {
		frames = append(frames, ms)
	}
	gen.Results[path] = &library.TrickplayResult{ImageCount: 4, FrameTimestampsMs: frames}
	svc := NewTrickplayService(repo, gen, t.TempDir(), media, sharedfakes.NewFakeEventBus(), nil)
	svc.SetTrickplaySettleForTest(probe, retry)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	svc.StartTrickplayWorker(ctx)
	return &settleFixture{svc: svc, gen: gen, repo: repo, media: media, path: path, mod: mod}
}

func (f *settleFixture) manifest() *library.TrickplayManifest {
	m, err := f.repo.GetByMediaItemID(context.Background(), "m1")
	if err != nil {
		return nil
	}
	return m
}

// A complete file, known to the library as it is on disk, is generated at
// once.
func TestTrickplaySettle_CompleteFileIsGenerated(t *testing.T) {
	f := newSettleFixture(t, 0, time.Hour)
	require.NoError(t, f.svc.GenerateTrickplay(context.Background(), "m1"))
	assert.EqualValues(t, 1, f.gen.calls.Load())
	assert.NotNil(t, f.manifest())
}

// A file whose size on disk is not the one the scan recorded is still being
// copied: no generation, no failure recorded, and the item is tried again
// later, when it goes through once the copy has ended.
func TestTrickplaySettle_SizeUnlikeTheLibraryIsPostponed(t *testing.T) {
	f := newSettleFixture(t, 0, 50*time.Millisecond)
	require.NoError(t, os.WriteFile(f.path, make([]byte, 400), 0o644))
	require.NoError(t, os.Chtimes(f.path, f.mod, f.mod))

	require.NoError(t, f.svc.GenerateTrickplay(context.Background(), "m1"))
	assert.Zero(t, f.gen.calls.Load(), "an incomplete file was decoded")
	assert.Empty(t, f.repo.Failures(), "a copy in progress is not a failure")

	require.NoError(t, os.WriteFile(f.path, make([]byte, 1000), 0o644))
	require.NoError(t, os.Chtimes(f.path, f.mod, f.mod))
	require.Eventually(t, func() bool { return f.manifest() != nil }, 5*time.Second, 10*time.Millisecond,
		"the postponed generation never came back")
}

// A file written less than a minute ago is not made sprites of, even when its
// size matches.
func TestTrickplaySettle_RecentlyModifiedIsPostponed(t *testing.T) {
	f := newSettleFixture(t, 0, time.Hour)
	now := time.Now()
	require.NoError(t, os.Chtimes(f.path, now, now))
	item, err := f.media.GetByID(context.Background(), "m1")
	require.NoError(t, err)
	item.File.ModTime = now
	require.NoError(t, f.media.Update(context.Background(), item))

	require.NoError(t, f.svc.GenerateTrickplay(context.Background(), "m1"))
	assert.Zero(t, f.gen.calls.Load())
	assert.Nil(t, f.manifest())
}

// A copy that keeps the source's timestamp looks settled; its size still
// moves during the probe.
func TestTrickplaySettle_GrowingUnderAnOldTimestampIsPostponed(t *testing.T) {
	f := newSettleFixture(t, 300*time.Millisecond, time.Hour)
	go func() {
		time.Sleep(50 * time.Millisecond)
		fh, err := os.OpenFile(f.path, os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			return
		}
		_, _ = fh.Write(make([]byte, 100))
		_ = fh.Close()
		_ = os.Chtimes(f.path, f.mod, f.mod)
	}()
	require.NoError(t, f.svc.GenerateTrickplay(context.Background(), "m1"))
	assert.Zero(t, f.gen.calls.Load(), "a growing file was decoded")
}

// The file changing on disk while it is read: the sprites are discarded, not
// stored, and the item is tried again.
func TestTrickplaySettle_ChangedDuringGenerationIsDiscarded(t *testing.T) {
	f := newSettleFixture(t, 0, time.Hour)
	f.gen.during = func() {
		require.NoError(t, os.WriteFile(f.path, make([]byte, 1500), 0o644))
	}
	require.NoError(t, f.svc.GenerateTrickplay(context.Background(), "m1"))
	assert.Nil(t, f.manifest(), "sprites of a file changing under them were stored")
	assert.Empty(t, f.repo.Failures())
}

// Thumbnails that stop well before the announced end of a complete file are
// kept, not refused: the picture can end there (prod Tron announces 156.5 min,
// its picture ends near 125, and refusing left it with no preview).
func TestTrickplaySettle_ShortCoverageOfACompleteFileIsKept(t *testing.T) {
	f := newSettleFixture(t, 0, time.Hour)
	f.gen.Results[f.path] = &library.TrickplayResult{ImageCount: 1, FrameTimestampsMs: []int64{0, 7000, 14000, 120000}}
	require.NoError(t, f.svc.GenerateTrickplay(context.Background(), "m1"))
	assert.NotNil(t, f.manifest())
	assert.Empty(t, f.repo.Failures())
}

func TestCheckTrickplayCoverage(t *testing.T) {
	const minute = int64(60_000)
	require.NoError(t, checkTrickplayCoverage([]int64{0, 54 * minute}, 55*minute), "a minute short of the end is complete")
	require.Error(t, checkTrickplayCoverage([]int64{0, 9 * minute}, 44*minute), "Raised by Wolves S02E04: 8:56 of 43:48")
	require.Error(t, checkTrickplayCoverage(nil, 44*minute), "no thumbnail at all")
	require.NoError(t, checkTrickplayCoverage([]int64{0}, 0), "an unknown duration is not judged")
	// Past twenty minutes, the allowance is a tenth of the media: 18 min on a
	// 3 h film.
	require.NoError(t, checkTrickplayCoverage([]int64{0, 165 * minute}, 180*minute))
	require.Error(t, checkTrickplayCoverage([]int64{0, 125 * minute}, 156*minute), "Tron: 125 of 156 min")
}
