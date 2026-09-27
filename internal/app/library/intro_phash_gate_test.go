package library

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

// The pHash intro pass is off by default since v6: with trickplay on and no sprites
// yet, an episode is no longer held back waiting for them. The setting brings it back.
func TestRunPipeline_PHashIntroPassIsOffByDefault(t *testing.T) {
	t.Parallel()
	item := &domainlibrary.MediaItem{ID: shared.MediaItemID("ep-1"), Type: domainlibrary.Episode, File: &domainlibrary.MediaFile{Path: "/tmp/x.mkv"}}
	run := func(introConfig string) error {
		values := map[string]string{"trickplay.enabled": "true"}
		if introConfig != "" {
			values[IntroDetectionConfigKey] = introConfig
		}
		svc := NewIntroDetectionService(&fakeSegmentRepo{}, &fakeTrickplayRepo{}, &fakeMediaItemRepo{}, &fakeConfigRepo{values: values}, &fakeDetector{}, t.TempDir())
		svc.trickplayWait = 20 * time.Millisecond
		_, err := svc.runPipeline(context.Background(), item, nil)
		return err
	}
	require.False(t, errors.Is(run(""), errTrickplayPending), "off by default: no wait for sprites")
	require.ErrorIs(t, run(`{"phash_intro_enabled":true}`), errTrickplayPending, "on: the pHash pass needs the sprites")
}
