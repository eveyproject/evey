package httpapi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Prod, 2026-09-09: five requests for the same PGS track all failed in the same
// millisecond with "context canceled". singleflight.Do runs the shared work on
// the WINNER's context, so the one client that hung up took the extraction - and
// every other waiter - down with it. Since the work is a full read of the film,
// nothing was cached either and the next attempt paid the whole price again.
func TestRunSharedSubtitleExtraction_AWaiterLeavingDoesNotKillTheWork(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var ranWith context.Context

	leaver, cancelLeaver := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Go(func() {
		_, err := runSharedSubtitleExtraction(leaver, "k", "", func(ctx context.Context) ([]byte, error) {
			ranWith = ctx
			close(started)
			<-release
			return []byte("extracted"), nil
		})
		// The caller that walked away gets its own cancellation, which is right.
		assert.ErrorIs(t, err, context.Canceled)
	})

	<-started
	cancelLeaver() // the winner disconnects mid-extraction
	wg.Wait()

	// The extraction must NOT have been cancelled with it.
	select {
	case <-ranWith.Done():
		t.Fatal("the shared extraction was cancelled by the caller that left")
	default:
	}
	close(release)
}

// The other half: a caller whose own context is alive gets the result, even
// though the flight was started by someone else.
func TestRunSharedSubtitleExtraction_SurvivingWaitersStillGetTheResult(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})

	var wg sync.WaitGroup
	got := make([][]byte, 3)
	errs := make([]error, 3)
	for i := range got {
		wg.Go(func() {
			got[i], errs[i] = runSharedSubtitleExtraction(context.Background(), "shared", "", func(context.Context) ([]byte, error) {
				select {
				case <-started:
				default:
					close(started)
				}
				<-release
				return []byte("blob"), nil
			})
		})
	}
	<-started
	close(release)
	wg.Wait()

	for i := range got {
		require.NoError(t, errs[i], "waiter %d", i)
		assert.Equal(t, []byte("blob"), got[i], "waiter %d", i)
	}
}

// An extraction that genuinely fails must still report its own error, not be
// masked by the detachment.
func TestRunSharedSubtitleExtraction_PropagatesTheWorkError(t *testing.T) {
	sentinel := errors.New("no such track")
	_, err := runSharedSubtitleExtraction(context.Background(), "boom", "", func(context.Context) ([]byte, error) {
		return nil, sentinel
	})
	assert.ErrorIs(t, err, sentinel)
}

// The detached context still has a deadline: a wedged extraction must not pin a
// full read of a film forever.
func TestRunSharedSubtitleExtraction_WorkContextIsBounded(t *testing.T) {
	var deadline time.Time
	var ok bool
	_, err := runSharedSubtitleExtraction(context.Background(), "deadline", "", func(ctx context.Context) ([]byte, error) {
		deadline, ok = ctx.Deadline()
		return []byte("x"), nil
	})
	require.NoError(t, err)
	require.True(t, ok, "the shared extraction must run under a deadline")
	assert.WithinDuration(t, time.Now().Add(subtitleExtractionTimeout), deadline, time.Minute)
}

// The whole point of the flight is that the ON-DEMAND handler and the PREWARMER
// collapse onto ONE extraction. They diverged once - the PGS handler spelled its
// key with an extra "|pgs|" marker the prewarmer did not - so a viewer asking for
// a bitmap track the prewarmer was already extracting started a SECOND full read
// of the film, on a network mount already measured at half its throughput.
// Nothing failed; it just read the film twice.
//
// Comparing two calls to subtitleFlightKey would prove nothing (it is a pure
// function). What has to hold is that every CALL SITE goes through it, so this
// reads the sources.
func TestSubtitleFlightKey_EveryCallSiteGoesThroughIt(t *testing.T) {
	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	// The flight is entered through exactly these two.
	callers := regexp.MustCompile(`(?:subtitleExtractions\.Do(?:Chan)?|runSharedSubtitleExtraction)\(`)
	checked := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		// subtitle_cache.go DEFINES both the key builder and the shared runner;
		// the runner necessarily enters the flight with a key it was handed.
		// Every OTHER file is a call site and must build the key properly.
		if name == "subtitle_cache.go" {
			continue
		}
		src, rerr := os.ReadFile(name)
		require.NoError(t, rerr)
		for line := range strings.SplitSeq(string(src), "\n") {
			if !callers.MatchString(line) {
				continue
			}
			checked++
			assert.Contains(t, line, "subtitleFlightKey(",
				"%s: the flight key must be built by subtitleFlightKey, or the handler and the "+
					"prewarmer stop sharing a flight and the film is read twice:\n\t%s", name, strings.TrimSpace(line))
		}
	}
	assert.GreaterOrEqual(t, checked, 3, "expected the three call sites (VTT handler, PGS handler, prewarmer)")

	// And the guard must be looking at something: a typo in the regex or a file
	// rename would otherwise make it pass by inspecting nothing.
	require.NotZero(t, checked)
}

// The two KINDS must still never share a flight: a text and a bitmap track of the
// same ordinal are different work producing different bytes.
func TestSubtitleFlightKey_KindAndTrackStayPartOfTheKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "film.mkv")
	require.NoError(t, os.WriteFile(path, []byte("not really a film"), 0o600))
	cache := subtitleCache{dir: dir}

	vtt := subtitleFlightKey(cache.keyForKind(path, 1, subtitleKindVTT), path)
	pgs := subtitleFlightKey(cache.keyForKind(path, 1, subtitleKindPGS), path)
	require.NotEmpty(t, vtt)
	require.NotEmpty(t, pgs)
	assert.NotEqual(t, vtt, pgs, "the kind must stay part of the key")

	other := subtitleFlightKey(cache.keyForKind(path, 2, subtitleKindPGS), path)
	assert.NotEqual(t, pgs, other, "different tracks, different flights")
}
