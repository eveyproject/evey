package library_test

import (
	"context"
	"testing"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A row whose streams were probed before the current prober revision keeps its
// streams (attempts == 0) yet must be re-probed by the next scan and stamped
// with the current StreamProbeVersion; a current row is left alone.
func TestScanService_StaleProbeVersion_IsReprobedOnce(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.scanner.Results["/m"] = []library.ScannedFile{
		{Path: "/m/a.mkv", Size: 100, Fingerprint: "fp", Type: library.MovieItem, Title: "A"},
	}
	h.prober.Results["/m/a.mkv"] = &library.ProbeResult{
		Duration: mustProbeDuration(42000),
		Streams:  []*library.MediaStream{{ID: "ps1", StreamType: "video", Codec: "vp9"}},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	items := h.media.All()
	require.Len(t, items, 1)
	assert.Equal(t, library.StreamProbeVersion, items[0].File.StreamProbeVersion, "a successful probe stamps the current version")
	assert.Zero(t, items[0].File.StreamProbeAttempts)

	// The prober learns a new field: simulate by changing its output.
	h.prober.Results["/m/a.mkv"] = &library.ProbeResult{
		Duration: mustProbeDuration(42000),
		Streams: []*library.MediaStream{
			{ID: "ps1", StreamType: "video", Codec: "vp9"},
			{ID: "ps2", StreamType: "audio", Codec: "aac"},
		},
	}
	_, _, err = h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	streams, err := h.streams.GetByMediaItemID(context.Background(), items[0].ID)
	require.NoError(t, err)
	assert.Len(t, streams, 1, "a row at the current version is not re-probed")

	// Row written before versioning (column default 0), streams already present.
	stale := h.media.All()[0]
	stale.File.StreamProbeVersion = 0
	require.NoError(t, h.media.Update(context.Background(), stale))

	_, _, err = h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	streams, err = h.streams.GetByMediaItemID(context.Background(), items[0].ID)
	require.NoError(t, err)
	assert.Len(t, streams, 2, "the stale-version row is re-probed")
	got := h.media.All()[0]
	assert.Equal(t, library.StreamProbeVersion, got.File.StreamProbeVersion)
	assert.Zero(t, got.File.StreamProbeAttempts, "attempts semantics untouched")
}
