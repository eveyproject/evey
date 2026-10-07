package library

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
)

type busyStreams struct{ n atomic.Int32 }

func (b *busyStreams) ActivePlaybackCount() int { return int(b.n.Load()) }

type countingGen struct {
	*libfakes.FakeTrickplayGenerator
	calls atomic.Int32
}

func (g *countingGen) Generate(ctx context.Context, filePath, outputDir string, interval, width, tileWidth, tileHeight, maxWorkers int, d int64) (*library.TrickplayResult, error) {
	g.calls.Add(1)
	return g.FakeTrickplayGenerator.Generate(ctx, filePath, outputDir, interval, width, tileWidth, tileHeight, maxWorkers, d)
}

// An item is not started while a playback is live, and is started as soon as
// the playback ends.
func TestGenerateTrickplay_StartsOnlyOnceTheViewerIsGone(t *testing.T) {
	old := trickplayIdlePoll
	trickplayIdlePoll = 10 * time.Millisecond
	defer func() { trickplayIdlePoll = old }()

	media := libfakes.NewFakeMediaItemCommandRepo()
	item := &library.MediaItem{ID: "media-busy", Title: "Film", File: &library.MediaFile{Path: "/media/films/z.mkv"}}
	require.NoError(t, media.Create(context.Background(), item))
	gen := &countingGen{FakeTrickplayGenerator: libfakes.NewFakeTrickplayGenerator()}
	svc := NewTrickplayService(libfakes.NewFakeTrickplayRepository(), gen, t.TempDir(), media, sharedfakes.NewFakeEventBus(), nil)
	streams := &busyStreams{}
	streams.n.Store(1)
	svc.WithActiveStreamChecker(streams)

	done := make(chan error, 1)
	go func() { done <- svc.GenerateTrickplay(context.Background(), "media-busy") }()

	time.Sleep(200 * time.Millisecond)
	require.Zero(t, gen.calls.Load(), "an item must not start while a playback is live")

	streams.n.Store(0)
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("generation did not start after the playback ended")
	}
	require.EqualValues(t, 1, gen.calls.Load())
}
