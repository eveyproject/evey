package library_test

import (
	"context"
	"errors"
	"testing"

	applib "github.com/eveyproject/evey/internal/app/library"
	"github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// updateFailsOnNth is the media item repository on the day the probe of a
// replaced file runs the worker's budget out: the Update after the probe
// fails, the one before it went through.
type updateFailsOnNth struct {
	*libfakes.FakeMediaItemCommandRepo
	calls, failOn int
}

func (r *updateFailsOnNth) Update(ctx context.Context, item *library.MediaItem) error {
	r.calls++
	if r.calls == r.failOn {
		return errors.New("begin tx: context deadline exceeded")
	}
	return r.FakeMediaItemCommandRepo.Update(ctx, item)
}

// A file replaced in place whose probe fails, and whose second Update fails
// with it, went out with the new fingerprint and the OLD file's streams: the
// next scan saw an unchanged fingerprint, no probe budget owed, and never
// probed the file again - every playback decision read the previous file.
// The first Update owes the re-probe, so the next scan settles it.
func TestScanService_ReplacedFileWhoseProbeAndUpdateFail_IsProbedOnTheNextScan(t *testing.T) {
	h := newScanHarness()
	media := &updateFailsOnNth{FakeMediaItemCommandRepo: h.media, failOn: 2}
	h.svc = applib.NewScanService(
		h.libCmd, h.libQuery, media, h.streams, h.persons, h.mediaPersons,
		h.scanner, h.prober, h.search, h.bus,
	)
	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/m"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)
	ctx := context.Background()
	require.NoError(t, h.media.Create(ctx, &library.MediaItem{
		ID: "item-1", LibraryID: "lib-1", Type: library.MovieItem, Title: "Film",
		File: &library.MediaFile{Path: "/m/film.mkv", Size: 500, Fingerprint: "fp-old", StreamProbeVersion: library.StreamProbeVersion},
	}))
	require.NoError(t, h.streams.CreateBatch(ctx, []*library.MediaStream{
		{ID: "s-old", MediaItemID: "item-1", StreamIndex: 0, StreamType: library.StreamTypeVideo, Codec: "h264"},
	}))

	// The upgrade lands: same path, new bytes, and the probe fails on it.
	h.scanner.Results["/m"] = []library.ScannedFile{{Path: "/m/film.mkv", Size: 1500, Fingerprint: "fp-new", Title: "Film", Type: library.MovieItem}}
	h.prober.Err = errors.New("probe timed out")
	_, _, err := h.svc.ScanLibrary(ctx, "lib-1")
	require.NoError(t, err)
	require.Equal(t, 2, media.calls, "premise: the second Update is the one that failed")

	stored, err := h.media.GetByID(ctx, "item-1")
	require.NoError(t, err)
	assert.Equal(t, "fp-new", stored.File.Fingerprint, "the first Update carried the new fingerprint")
	assert.Equal(t, 1, stored.File.StreamProbeAttempts, "the first Update owes the re-probe the failed one never settled")
	oldStreams, err := h.streams.GetByMediaItemID(ctx, "item-1")
	require.NoError(t, err)
	require.Len(t, oldStreams, 1)
	assert.Equal(t, "h264", oldStreams[0].Codec, "premise: the old file's streams are still on the row")

	// The next scan: nothing changed on disk, the probe answers.
	h.prober.Err = nil
	h.prober.Results["/m/film.mkv"] = &library.ProbeResult{Streams: []*library.MediaStream{
		{StreamIndex: 0, StreamType: library.StreamTypeVideo, Codec: "hevc"},
	}}
	probesBefore := h.prober.Calls.Load()
	_, _, err = h.svc.ScanLibrary(ctx, "lib-1")
	require.NoError(t, err)
	assert.Greater(t, h.prober.Calls.Load(), probesBefore, "an unchanged file with a probe owed is probed again")
	newStreams, err := h.streams.GetByMediaItemID(ctx, "item-1")
	require.NoError(t, err)
	require.Len(t, newStreams, 1)
	assert.Equal(t, "hevc", newStreams[0].Codec, "the new file's streams replace the old file's")
	stored, err = h.media.GetByID(ctx, "item-1")
	require.NoError(t, err)
	assert.Equal(t, 0, stored.File.StreamProbeAttempts, "the settled probe clears the budget")
}
