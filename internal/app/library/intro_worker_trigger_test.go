package library

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
)

// The event path of the worker: a movie whose trickplay was just generated is
// detected right away (no siblings to wait for), and not while someone is
// watching - a detection decodes on top of the live stream on a weak NAS.
func TestStartWorker_MovieTrickplayTriggersDetection(t *testing.T) {
	movie := &domainlibrary.MediaItem{
		ID:   shared.MediaItemID("movie-1"),
		Type: domainlibrary.MovieItem,
		File: &domainlibrary.MediaFile{Path: "/tmp/movie.mkv"},
	}
	start := func(t *testing.T, watching int) *countingDetector {
		t.Helper()
		det := &countingDetector{fakeDetector: fakeDetector{leadingLogoFound: true, leadingLogoEndMs: 35_000}}
		svc := NewIntroDetectionService(&fakeSegmentRepo{}, &fakeTrickplayRepo{}, &fakeMediaItemRepo{item: movie},
			&fakeConfigRepo{}, det, "/tmp/trickplay")
		svc.backfillBootDelay = time.Hour // keep the catalogue sweep out of this test
		svc.WithActivePlayback(func() int { return watching })
		bus := sharedfakes.NewFakeEventBus()
		svc.WithEventBus(bus)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		svc.StartWorker(ctx)
		require.NoError(t, bus.Publish(ctx, domainlibrary.TrickplayGenerated{
			MediaItemID: movie.ID, Type: domainlibrary.MovieItem, OccurredAt: time.Now(),
		}))
		return det
	}

	t.Run("idle server: the movie is detected", func(t *testing.T) {
		det := start(t, 0)
		require.Eventually(t, func() bool { return det.leadingLogoCalls.Load() > 0 },
			5*time.Second, 10*time.Millisecond, "trickplay.generated on a movie must run its detection")
	})

	t.Run("playback running: the detection waits", func(t *testing.T) {
		det := start(t, 1)
		require.Never(t, func() bool { return det.leadingLogoCalls.Load() > 0 },
			300*time.Millisecond, 10*time.Millisecond, "no detection may decode while a stream is playing")
	})
}
