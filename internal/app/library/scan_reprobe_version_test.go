package library_test

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/time/rate"

	"github.com/eveyproject/evey/internal/domain/shared"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A row whose streams were probed before the current prober revision keeps its
// streams (attempts == 0) yet must be re-probed by the next scan and stamped
// with the current StreamProbeVersion; a current row is left alone. A re-probe
// that finds other streams than the stored ones writes none: it refreshes
// fields in place, it never replaces rows.
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
	assert.Len(t, streams, 1, "another stream set is not written by a metadata refresh")
	assert.Equal(t, int64(2), h.prober.Calls.Load(), "the stale-version row is re-probed")
	got := h.media.All()[0]
	assert.Equal(t, library.StreamProbeVersion, got.File.StreamProbeVersion)
	assert.Zero(t, got.File.StreamProbeAttempts, "attempts semantics untouched")
}

// An unchanged file ffprobe always rejects is probed a bounded number of times,
// not once per periodic scan: neither the streamless retries nor the stale-
// version refresh may turn into a ffprobe every two minutes forever.
func TestScanService_UnprobeableFile_StopsBeingProbed(t *testing.T) {
	for _, tc := range []struct {
		name     string
		hadProbe bool // probed fine once, by an older prober revision
	}{{"never probed", false}, {"probed by an older revision", true}} {
		t.Run(tc.name, func(t *testing.T) {
			h := newScanHarness()
			h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
			h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
			h.scanner.Results["/m"] = []library.ScannedFile{
				{Path: "/m/a.mkv", Size: 100, Fingerprint: "fp", Type: library.MovieItem, Title: "A"},
			}
			if tc.hadProbe {
				h.prober.Results["/m/a.mkv"] = &library.ProbeResult{
					Duration: mustProbeDuration(42000),
					Streams:  []*library.MediaStream{{ID: "ps1", StreamType: "video", Codec: "vp9"}},
				}
				_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
				require.NoError(t, err)
				stale := h.media.All()[0]
				stale.File.StreamProbeVersion = 0
				require.NoError(t, h.media.Update(context.Background(), stale))
			}
			h.prober.Err = errors.New("ffprobe: exit status 1")
			h.prober.Calls.Store(0)

			for range 20 {
				_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
				require.NoError(t, err)
			}
			assert.LessOrEqual(t, h.prober.Calls.Load(), int64(2), "an unchanged unprobeable file is not probed on every scan")

			if tc.hadProbe {
				streams, err := h.streams.GetByMediaItemID(context.Background(), h.media.All()[0].ID)
				require.NoError(t, err)
				assert.Len(t, streams, 1, "a failed refresh keeps the streams it had")
			}

			// A changed file is probed again.
			h.scanner.Results["/m"][0].Fingerprint = "fp2"
			h.prober.Calls.Store(0)
			_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
			require.NoError(t, err)
			assert.Equal(t, int64(1), h.prober.Calls.Load(), "a changed file is probed again")
		})
	}
}

// The whole-library metadata refresh a StreamProbeVersion bump runs: an
// unchanged file gets the frame rate and colour the new prober reads, while
// everything the backfills derived on its streams stays - so no level, idet,
// frame-rate or keyframe worker goes back over the file - and nothing is
// published, so no trickplay, intro, chapter or cache work follows either.
func TestScanService_StaleProbeVersion_RefreshesMetadataOnly(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.scanner.Results["/m"] = []library.ScannedFile{
		{Path: "/m/a.mkv", Size: 100, Fingerprint: "fp", Type: library.MovieItem, Title: "A"},
	}
	h.prober.Results["/m/a.mkv"] = &library.ProbeResult{
		Duration: mustProbeDuration(42000),
		Streams: []*library.MediaStream{
			{ID: "v", StreamIndex: 0, StreamType: "video", Codec: "hevc"},
			{ID: "a", StreamIndex: 1, StreamType: "audio", Codec: "eac3"},
		},
	}
	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	item := h.media.All()[0]

	// What the backfills wrote on the video row since.
	streams, err := h.streams.GetByMediaItemID(context.Background(), item.ID)
	require.NoError(t, err)
	level, interlaced := 153, false
	streams[0].Level, streams[0].IdetInterlaced, streams[0].FrameRateMode = &level, &interlaced, "cfr"

	// The prober learns the frame rate and the colour; the row predates it.
	stale := h.media.All()[0]
	stale.File.StreamProbeVersion = 0
	require.NoError(t, h.media.Update(context.Background(), stale))
	fps, transfer := 23.976, "smpte2084"
	h.prober.Results["/m/a.mkv"] = &library.ProbeResult{
		Duration: mustProbeDuration(42000),
		Streams: []*library.MediaStream{
			{ID: "v2", StreamIndex: 0, StreamType: "video", Codec: "hevc", Framerate: &fps, ColorTransfer: &transfer},
			{ID: "a2", StreamIndex: 1, StreamType: "audio", Codec: "eac3"},
		},
	}
	published := len(h.bus.PublishedEvents())

	_, _, err = h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)

	streams, err = h.streams.GetByMediaItemID(context.Background(), item.ID)
	require.NoError(t, err)
	require.Len(t, streams, 2)
	v := streams[0]
	assert.Equal(t, "v", v.ID, "the row is refreshed in place, not replaced")
	require.NotNil(t, v.Framerate)
	assert.InDelta(t, 23.976, *v.Framerate, 1e-9)
	require.NotNil(t, v.ColorTransfer)
	assert.Equal(t, "smpte2084", *v.ColorTransfer)
	require.NotNil(t, v.Level)
	assert.Equal(t, 153, *v.Level, "the level backfill's value stays")
	require.NotNil(t, v.IdetInterlaced)
	assert.False(t, *v.IdetInterlaced, "the idet verdict stays")
	assert.Equal(t, "cfr", v.FrameRateMode, "the frame-rate classification stays")
	assert.Equal(t, library.StreamProbeVersion, h.media.All()[0].File.StreamProbeVersion)
	for _, ev := range h.bus.PublishedEvents()[published:] {
		switch ev.(type) {
		case library.MediaItemUpdated, library.MediaItemAdded:
			t.Errorf("a metadata refresh published %T, which sends workers over the file", ev)
		}
	}
}

// The version re-probes are paced: a scan re-probes as many stale rows as the
// pace allows and leaves the others unstamped, and a later scan takes them.
// Nothing is lost when a scan stops halfway, nothing is probed twice.
func TestScanService_StaleProbeVersion_IsPacedAndResumes(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	paths := []string{"/m/a.mkv", "/m/b.mkv", "/m/c.mkv"}
	for _, p := range paths {
		h.scanner.Results["/m"] = append(h.scanner.Results["/m"],
			library.ScannedFile{Path: p, Size: 100, Fingerprint: "fp" + p, Type: library.MovieItem, Title: p})
		h.prober.Results[p] = &library.ProbeResult{
			Duration: mustProbeDuration(42000),
			Streams:  []*library.MediaStream{{ID: "v" + p, StreamType: "video", Codec: "h264"}},
		}
	}
	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	for _, it := range h.media.All() {
		it.File.StreamProbeVersion = 0
		require.NoError(t, h.media.Update(context.Background(), it))
	}
	h.prober.Calls.Store(0)

	// One re-probe allowed, none refilled during the test.
	h.svc.WithStaleReprobePace(0, 1)
	_, _, err = h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, int64(1), h.prober.Calls.Load(), "one file re-probed, at the pace")
	stale := 0
	for _, it := range h.media.All() {
		if it.File.StreamProbeVersion < library.StreamProbeVersion {
			stale++
		}
	}
	assert.Equal(t, 2, stale, "the files over the pace stay unstamped")

	// The next scans take the rest, each file once.
	h.svc.WithStaleReprobePace(rate.Inf, 1)
	_, _, err = h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	_, _, err = h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, int64(3), h.prober.Calls.Load(), "every stale file re-probed exactly once")
	for _, it := range h.media.All() {
		assert.Equal(t, library.StreamProbeVersion, it.File.StreamProbeVersion)
	}
}

// A version re-probe skips a file a play session is open on: a refreshed frame
// rate would move its segment grid and get the player's URLs refused 410
// mid-film (an error screen on the installed phone and TV builds). The file is
// taken by a scan after the session ends. Sessions that cannot be read hold
// every re-probe back.
func TestScanService_StaleProbeVersion_SkipsFilesBeingWatched(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	for _, p := range []string{"/m/watched.mkv", "/m/idle.mkv"} {
		h.scanner.Results["/m"] = append(h.scanner.Results["/m"],
			library.ScannedFile{Path: p, Size: 100, Fingerprint: "fp" + p, Type: library.MovieItem, Title: p})
		h.prober.Results[p] = &library.ProbeResult{
			Duration: mustProbeDuration(42000),
			Streams:  []*library.MediaStream{{ID: "v" + p, StreamType: "video", Codec: "h264"}},
		}
	}
	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	var watched shared.MediaItemID
	for _, it := range h.media.All() {
		if it.File.Path == "/m/watched.mkv" {
			watched = it.ID
		}
		it.File.StreamProbeVersion = 0
		require.NoError(t, h.media.Update(context.Background(), it))
	}
	require.NotEmpty(t, watched)
	version := func(path string) int {
		for _, it := range h.media.All() {
			if it.File.Path == path {
				return it.File.StreamProbeVersion
			}
		}
		t.Fatalf("no item at %s", path)
		return 0
	}

	// Sessions unreadable: nothing is re-probed.
	h.prober.Calls.Store(0)
	h.svc.WithPlayingMedia(func(context.Context) (map[shared.MediaItemID]bool, error) {
		return nil, errors.New("db down")
	})
	_, _, err = h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Zero(t, h.prober.Calls.Load(), "no session list, no re-probe")

	// One file is being watched: the other one is re-probed, it is not.
	h.svc.WithPlayingMedia(func(context.Context) (map[shared.MediaItemID]bool, error) {
		return map[shared.MediaItemID]bool{watched: true}, nil
	})
	_, _, err = h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, int64(1), h.prober.Calls.Load())
	assert.Equal(t, library.StreamProbeVersion, version("/m/idle.mkv"))
	assert.Less(t, version("/m/watched.mkv"), library.StreamProbeVersion, "the watched file is left for later")

	// The session ends: the next scan takes it.
	h.svc.WithPlayingMedia(func(context.Context) (map[shared.MediaItemID]bool, error) {
		return map[shared.MediaItemID]bool{}, nil
	})
	_, _, err = h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, int64(2), h.prober.Calls.Load())
	assert.Equal(t, library.StreamProbeVersion, version("/m/watched.mkv"))
}
