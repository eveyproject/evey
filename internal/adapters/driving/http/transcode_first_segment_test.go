package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/metrics"
)

func firstSegmentSamples(t *testing.T, servedBy string) uint64 {
	t.Helper()
	var m dto.Metric
	h, ok := metrics.HLSFirstSegmentSeconds.WithLabelValues(servedBy).(prometheus.Histogram)
	require.True(t, ok)
	require.NoError(t, h.Write(&m))
	return m.GetHistogram().GetSampleCount()
}

func TestFirstSegmentClock_TakenOnceAndExpires(t *testing.T) {
	var c firstSegmentClock
	t0 := time.Now()

	_, ok := c.take("k", t0)
	assert.False(t, ok, "no master, no startup to measure")

	c.arm("k", t0)
	d, ok := c.take("k", t0.Add(1500*time.Millisecond))
	require.True(t, ok)
	assert.Equal(t, 1500*time.Millisecond, d)
	_, ok = c.take("k", t0.Add(2*time.Second))
	assert.False(t, ok, "only the first segment of a playback is its startup")

	c.arm("k", t0)
	_, ok = c.take("k", t0.Add(firstSegmentTTL+time.Second))
	assert.False(t, ok, "a master never followed by a segment is not a startup")
}

// TestHLSVOD_FirstSegmentAfterMasterIsObservedOnce: the histogram had no
// source at all - a slow start (cold encoder, saturated disk) showed nowhere
// but in the viewer's complaint.
func TestHLSVOD_FirstSegmentAfterMasterIsObservedOnce(t *testing.T) {
	dur, _ := shared.NewDurationMs(60_000)
	item := &domainlibrary.MediaItem{
		ID:        "m1",
		LibraryID: "lib1",
		File:      &domainlibrary.MediaFile{Path: "/test.mkv", Duration: dur},
	}
	h, cacheDir := buildVODHandler(t, &testFakeStreamsGetter{item: item}, allowAll(), testFakeParentalChecker{})
	fp := apptranscoding.DecisionFingerprint{
		MediaID:             "m1",
		SourcePath:          "/test.mkv",
		SubtitleBurnInTrack: -1,
		UseFMP4:             true,
		Profiles:            profilesForTest(),
	}
	segDir := filepath.Join(cacheDir, "vod", "segments", fp.Hash())
	require.NoError(t, os.MkdirAll(segDir, 0o755))
	seg := []byte{
		0, 0, 0, 0x10, 's', 't', 'y', 'p', 'm', 's', 'd', 'h', 0, 0, 0, 0,
		0, 0, 0, 0x08, 'm', 'o', 'o', 'f',
		0, 0, 0, 0x0C, 'm', 'd', 'a', 't', 0, 0, 0, 0,
	}
	require.NoError(t, os.WriteFile(filepath.Join(segDir, "stream_0_000.m4s"), seg, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(segDir, "stream_0_001.m4s"), seg, 0o644))
	segment := func(file string) int {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/v2/"+file, nil)
		req.SetPathValue("id", "m1")
		req.SetPathValue("file", file)
		req = withHTTPClaims(req, testClaims(false))
		w := httptest.NewRecorder()
		h.serveHTTP(w, req)
		return w.Code
	}

	before := firstSegmentSamples(t, "transcode")
	require.Equal(t, http.StatusOK, doVODRequest(h, "m1", "master.m3u8").Code)
	require.Equal(t, http.StatusOK, segment("stream_0_000.m4s"))
	assert.Equal(t, before+1, firstSegmentSamples(t, "transcode"), "the first segment after the master is the startup")
	require.Equal(t, http.StatusOK, segment("stream_0_001.m4s"))
	assert.Equal(t, before+1, firstSegmentSamples(t, "transcode"), "later segments are not")
}
