package httpapi

import (
	"context"
	"errors"
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
		_, err := runSharedSubtitleExtraction(leaver, "k", func(ctx context.Context) ([]byte, error) {
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
			got[i], errs[i] = runSharedSubtitleExtraction(context.Background(), "shared", func(context.Context) ([]byte, error) {
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
	_, err := runSharedSubtitleExtraction(context.Background(), "boom", func(context.Context) ([]byte, error) {
		return nil, sentinel
	})
	assert.ErrorIs(t, err, sentinel)
}

// The detached context still has a deadline: a wedged extraction must not pin a
// full read of a film forever.
func TestRunSharedSubtitleExtraction_WorkContextIsBounded(t *testing.T) {
	var deadline time.Time
	var ok bool
	_, err := runSharedSubtitleExtraction(context.Background(), "deadline", func(ctx context.Context) ([]byte, error) {
		deadline, ok = ctx.Deadline()
		return []byte("x"), nil
	})
	require.NoError(t, err)
	require.True(t, ok, "the shared extraction must run under a deadline")
	assert.WithinDuration(t, time.Now().Add(subtitleExtractionTimeout), deadline, time.Minute)
}
