package library_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/domain/library"
)

type fakeContainerChecker struct {
	damaged map[string]int64
	err     error
	calls   atomic.Int64
}

func (f *fakeContainerChecker) ProbeContainerDamage(_ context.Context, path string) (int64, error) {
	f.calls.Add(1)
	if f.err != nil {
		return 0, f.err
	}
	return f.damaged[path], nil
}

func containerCheckHarness(c *fakeContainerChecker) *scanHarness {
	h := newScanHarness()
	h.svc.WithContainerChecker(c)
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.scanner.Results["/m"] = []library.ScannedFile{
		{Path: "/m/a.mkv", Size: 100, Fingerprint: "fp", Type: library.MovieItem, Title: "A"},
	}
	h.prober.Results["/m/a.mkv"] = &library.ProbeResult{
		Duration: mustProbeDuration(42000),
		Streams:  []*library.MediaStream{{ID: "ps1", StreamType: "video", Codec: "h264"}},
	}
	return h
}

// A file added with a damaged container is known as such from its first
// scan: the playback gate refuses to hand it over whole before any device
// has to fail on it.
func TestScanService_ContainerCheck_OnANewFile(t *testing.T) {
	c := &fakeContainerChecker{damaged: map[string]int64{"/m/a.mkv": 6}}
	h := containerCheckHarness(c)

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)

	f := h.media.All()[0].File
	assert.EqualValues(t, 6, f.ContainerDamagedBytes)
	assert.Equal(t, library.ContainerCheckVersion, f.ContainerCheckVersion)
	assert.True(t, f.RefusesDirectPlay())

	// A later scan of the unchanged file does not check it again.
	_, _, err = h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.EqualValues(t, 1, c.calls.Load())
}

// The files already in the library (added before the check existed) are
// checked once, the check alone, without re-probing their streams.
func TestScanService_ContainerCheck_CatchesUpOnTheLibrary(t *testing.T) {
	h := containerCheckHarness(&fakeContainerChecker{})
	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	probes := h.prober.Calls.Load()

	old := h.media.All()[0]
	old.File.ContainerCheckVersion = 0
	require.NoError(t, h.media.Update(context.Background(), old))
	c := &fakeContainerChecker{damaged: map[string]int64{"/m/a.mkv": 6}}
	h.svc.WithContainerChecker(c)

	_, _, err = h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)

	f := h.media.All()[0].File
	assert.EqualValues(t, 6, f.ContainerDamagedBytes)
	assert.Equal(t, library.ContainerCheckVersion, f.ContainerCheckVersion)
	assert.EqualValues(t, 1, c.calls.Load())
	assert.Equal(t, probes, h.prober.Calls.Load(), "the streams are not probed again for it")
}

// A check that fails is no clearance: the row stays owed a check and keeps
// what it knew.
func TestScanService_ContainerCheck_FailureKeepsItOwed(t *testing.T) {
	c := &fakeContainerChecker{err: errors.New("read: input/output error")}
	h := containerCheckHarness(c)

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)

	f := h.media.All()[0].File
	assert.Zero(t, f.ContainerCheckVersion, "still owed a check")
	assert.Zero(t, f.ContainerDamagedBytes)

	c.err = nil
	c.damaged = map[string]int64{"/m/a.mkv": 6}
	_, _, err = h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.EqualValues(t, 6, h.media.All()[0].File.ContainerDamagedBytes, "the next scan checks it")
}
