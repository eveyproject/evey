package library

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

// Durability of the sweep. Its intention used to live in s.backfillInFlight, a
// map in memory: a redeployment lost it and nothing picked it back up (prod
// 2026-08-09, 42h of sweep killed by a restart, never resumed, 31 detection
// decisions in the 11 days that followed). The intention now comes from the
// database, like the keyframe backfill that resumed on its own 10 days after a
// restart.

// resumeFixture wires N seasons of one episode each, none covered, plus the
// season→episode map the fake needs to derive the work list from its own state.
func resumeFixture(t *testing.T, seasons int) (*fakeSegmentRepo, *fakeMediaItemRepo) {
	t.Helper()
	repo := &fakeSegmentRepo{episodesBySeason: map[string][]shared.MediaItemID{}}
	var eps []*domainlibrary.MediaItem
	for i := range seasons {
		seasonID := fmt.Sprintf("season-%d", i)
		epID := shared.MediaItemID(fmt.Sprintf("ep-%d", i))
		idx := 1
		eps = append(eps, &domainlibrary.MediaItem{
			ID: epID, Type: domainlibrary.Episode, ParentID: seasonID, IndexNumber: &idx,
			File: &domainlibrary.MediaFile{Path: "/tmp/x.mkv", Duration: mustDurationMs(t, costTestDurMs)},
		})
		repo.episodesBySeason[seasonID] = []shared.MediaItemID{epID}
	}
	return repo, &fakeMediaItemRepo{siblings: eps}
}

func resumeService(repo *fakeSegmentRepo, items *fakeMediaItemRepo) *IntroDetectionService {
	svc := NewIntroDetectionService(repo, &fakeTrickplayRepo{}, items, &fakeConfigRepo{}, &countingDetector{}, "/tmp/trickplay")
	svc.backfillBootDelay = time.Millisecond
	svc.backfillIdle = time.Millisecond
	return svc
}

// A sweep cut short by a redeployment must be picked back up by the NEXT boot,
// without anyone re-triggering it: the remaining work is a SQL question, and a
// fresh service asking it gets the seasons the dead process never reached.
func TestIntroBackfill_ResumesAfterARestart(t *testing.T) {
	t.Parallel()
	repo, items := resumeFixture(t, 6)

	// First process: killed after its very first season, exactly like a
	// redeployment landing mid-sweep.
	killed, cancel := context.WithCancel(context.Background())
	repo.onPendingListed = func(int) { cancel() }
	resumeService(repo, items).sweepPendingSeasons(killed)
	repo.onPendingListed = nil

	pendingAfterCrash, err := repo.ListSeasonsPendingIntroDetection(
		context.Background(), introDetectionAlgoVersion, maxIntroDetectionAttempts, 100, nil)
	require.NoError(t, err)
	require.NotEmpty(t, pendingAfterCrash, "the crash must leave work behind, or the test proves nothing")

	require.Empty(t, repo.attempts, "the killed sweep reached no episode, so it left all the work behind")

	// Second process: a brand new service over the same database, no shared
	// memory, nobody pressing a button.
	restarted := resumeService(repo, items)
	swept := restarted.sweepPendingSeasons(context.Background())

	assert.Equal(t, len(pendingAfterCrash), swept,
		"the restarted sweep must cover everything the killed one left behind")
	assert.Len(t, repo.attempts, 6, "every episode the dead process never reached was detected")

	// And the work list is a cursor, not an infinite queue: a second cycle
	// spends the last retry of each episode and the catalogue goes quiet.
	restarted.sweepPendingSeasons(context.Background())
	remaining, err := repo.ListSeasonsPendingIntroDetection(
		context.Background(), introDetectionAlgoVersion, maxIntroDetectionAttempts, 100, nil)
	require.NoError(t, err)
	assert.Empty(t, remaining,
		"once every episode burned its retry budget at the current algo version the sweep goes idle")
}

// The drain must terminate. A season the sweep walked without making progress
// (nothing detectable, no attempt recorded) is still pending afterwards: without
// an in-run skip set the next batch hands back the same ids forever.
func TestIntroBackfill_DrainStopsOnSeasonsThatMakeNoProgress(t *testing.T) {
	t.Parallel()
	repo, items := resumeFixture(t, 3)
	// Detection records nothing: every season stays pending after its sweep.
	repo.dropAttempts = true
	svc := resumeService(repo, items)

	done := make(chan int, 1)
	go func() { done <- svc.sweepPendingSeasons(context.Background()) }()
	select {
	case swept := <-done:
		assert.Equal(t, 3, swept, "each pending season is walked exactly once per drain cycle")
	case <-time.After(5 * time.Second):
		t.Fatal("the drain span on seasons that make no progress instead of ending the cycle")
	}
}

// The per-library anti-duplicate guard has to survive the sweep starting itself:
// a catalogue-wide sweep covers every library, so an admin trigger landing on
// top of it would be a second sweep over the same seasons.
func TestIntroBackfill_SweepBlocksAConcurrentAdminBackfill(t *testing.T) {
	t.Parallel()
	repo, items := resumeFixture(t, 2)
	svc := resumeService(repo, items)

	held := make(chan struct{})
	release := make(chan struct{})
	repo.onPendingListed = func(int) {
		select {
		case <-held:
		default:
			close(held)
			<-release
		}
	}
	swept := make(chan int, 1)
	go func() { swept <- svc.sweepPendingSeasons(context.Background()) }()
	<-held

	_, err := svc.BackfillLibrary(context.Background(), "lib-1")
	close(release)
	<-swept
	assert.ErrorIs(t, err, ErrIntroBackfillAlreadyRunning,
		"an admin backfill must not stack a second sweep on top of the automatic one")
}

// The sweep is a loop, not a one-shot: once the work list comes back empty it
// sleeps and asks again, so seasons scanned later are picked up without a
// restart - and it stops when the server shuts down.
func TestIntroBackfill_LoopAsksAgainAfterAnEmptyListAndStopsOnShutdown(t *testing.T) {
	t.Parallel()
	repo, items := resumeFixture(t, 2)
	listed := make(chan int, 64)
	repo.onPendingListed = func(n int) {
		select {
		case listed <- n:
		default:
		}
	}
	svc := resumeService(repo, items)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		svc.runResumableBackfill(ctx)
		close(done)
	}()

	sawEmpty := false
	deadline := time.After(5 * time.Second)
	for asked := false; !asked; {
		select {
		case n := <-listed:
			if sawEmpty {
				asked = true
			}
			if n == 0 {
				sawEmpty = true
			}
		case <-deadline:
			cancel()
			t.Fatalf("the sweep never asked again after an empty work list (saw empty: %v)", sawEmpty)
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the sweep kept running after its context was cancelled")
	}
}
