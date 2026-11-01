package grpcapi

import (
	"context"
	"io/fs"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeClock struct{ t atomic.Int64 }

func (c *fakeClock) now() time.Time          { return time.Unix(0, c.t.Load()) }
func (c *fakeClock) advance(d time.Duration) { c.t.Add(int64(d)) }

func TestLibraryRescanner_CooldownAfterAScanFinishes(t *testing.T) {
	clock := &fakeClock{}
	clock.advance(time.Hour)
	var scans atomic.Int32
	done := make(chan struct{}, 8)
	rescan := newLibraryRescanner(func(context.Context, string) error {
		scans.Add(1)
		done <- struct{}{}
		return nil
	}, 5*time.Minute, clock.now)

	rescan(context.Background(), "lib-1")
	<-done
	// Let the goroutine record the finish before the next request.
	require.Eventually(t, func() bool {
		rescan(context.Background(), "lib-1")
		return true
	}, time.Second, time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	assert.EqualValues(t, 1, scans.Load(), "a request inside the cooldown must not start another scan")

	rescan(context.Background(), "lib-2")
	<-done
	assert.EqualValues(t, 2, scans.Load(), "the cooldown is per library")

	clock.advance(6 * time.Minute)
	rescan(context.Background(), "lib-1")
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("no scan after the cooldown elapsed")
	}
	assert.EqualValues(t, 3, scans.Load())
}

func TestMediaFileExists_HungStatCountsAsPresent(t *testing.T) {
	release := make(chan struct{})
	prevStat, prevTimeout := statFile, statTimeout
	statFile = func(string) (os.FileInfo, error) { <-release; return nil, fs.ErrNotExist }
	statTimeout = 30 * time.Millisecond
	defer func() { statFile, statTimeout = prevStat, prevTimeout; close(release) }()

	start := time.Now()
	assert.True(t, mediaFileExists("/hung/mount/file.mkv"), "an unknown answer must let playback proceed, not trigger a rescan")
	assert.Less(t, time.Since(start), time.Second)
}

func TestMediaFileExists_ProvenAbsenceStillReported(t *testing.T) {
	assert.False(t, mediaFileExists(t.TempDir()+"/gone.mkv"))
	assert.True(t, mediaFileExists(t.TempDir()))
}

func TestMediaFileExists_OnePendingStatPerPath(t *testing.T) {
	release := make(chan struct{})
	var calls atomic.Int32
	prevStat, prevTimeout := statFile, statTimeout
	statFile = func(string) (os.FileInfo, error) { calls.Add(1); <-release; return nil, nil }
	statTimeout = 10 * time.Millisecond
	defer func() { statFile, statTimeout = prevStat, prevTimeout; close(release) }()

	for range 5 {
		mediaFileExists("/hung/same.mkv")
	}
	assert.EqualValues(t, 1, calls.Load())
}
