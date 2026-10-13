package library

import (
	"context"
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

// replacingGen replaces the item's file (as the scan would) while its first
// generation reads it, when replace is set.
type replacingGen struct {
	*libfakes.FakeTrickplayGenerator
	media   *libfakes.FakeMediaItemCommandRepo
	replace bool
	calls   atomic.Int32
}

func (g *replacingGen) Generate(ctx context.Context, filePath, outputDir string, interval, width, tileWidth, tileHeight, maxWorkers int, d int64) (*library.TrickplayResult, error) {
	if g.calls.Add(1) == 1 && g.replace {
		item, err := g.media.GetByID(ctx, "media-repl")
		if err != nil {
			return nil, err
		}
		item.File.Size, item.File.ModTime = 200, item.File.ModTime.Add(time.Hour)
		if err := g.media.Update(ctx, item); err != nil {
			return nil, err
		}
	}
	return g.FakeTrickplayGenerator.Generate(ctx, filePath, outputDir, interval, width, tileWidth, tileHeight, maxWorkers, d)
}

func replacedDuringFixture(t *testing.T, replace bool) (*TrickplayService, *replacingGen, *libfakes.FakeTrickplayRepository) {
	t.Helper()
	media := libfakes.NewFakeMediaItemCommandRepo()
	mod := time.Date(2026, 10, 13, 10, 0, 0, 0, time.UTC)
	require.NoError(t, media.Create(context.Background(), &library.MediaItem{
		ID: "media-repl", Type: library.MovieItem, Title: "Film",
		File: &library.MediaFile{Path: "/media/films/r.mkv", Size: 100, ModTime: mod},
	}))
	repo := libfakes.NewFakeTrickplayRepository()
	repo.CurrentFile = func(id shared.MediaItemID) (int64, time.Time, bool) {
		it, err := media.GetByID(context.Background(), id)
		if err != nil || it.File == nil {
			return 0, time.Time{}, false
		}
		return it.File.Size, it.File.ModTime, true
	}
	gen := &replacingGen{FakeTrickplayGenerator: libfakes.NewFakeTrickplayGenerator(), media: media, replace: replace}
	svc := NewTrickplayService(repo, gen, t.TempDir(), media, sharedfakes.NewFakeEventBus(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	svc.StartTrickplayWorker(ctx)
	return svc, gen, repo
}

// A file replaced while its sprites were being made: the generation ends with
// a manifest of the old file, so the item goes straight back to the queue and
// a second generation makes the new file's sprites.
func TestGenerateTrickplay_FileReplacedDuringGenerationIsRemadeAtOnce(t *testing.T) {
	svc, gen, repo := replacedDuringFixture(t, true)
	require.NoError(t, svc.GenerateTrickplay(context.Background(), "media-repl"))

	require.Eventually(t, func() bool { return gen.calls.Load() == 2 }, 5*time.Second, 10*time.Millisecond,
		"the replaced file waited for the next sweep")
	require.Eventually(t, func() bool {
		m, err := repo.GetByMediaItemID(context.Background(), "media-repl")
		return err == nil && m.FileSize == 200
	}, 5*time.Second, 10*time.Millisecond, "the served sprites are not the new file's")
}

// An untouched file is generated once.
func TestGenerateTrickplay_UnchangedFileIsNotRemade(t *testing.T) {
	svc, gen, _ := replacedDuringFixture(t, false)
	require.NoError(t, svc.GenerateTrickplay(context.Background(), "media-repl"))
	time.Sleep(150 * time.Millisecond)
	assert.EqualValues(t, 1, gen.calls.Load())
}
