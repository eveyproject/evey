package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// rewritingProducer opens a `.tmp` next to the requested segment, as a second
// encoder rewriting a segment already on disk does. SegmentCache.IsReady reads
// that sibling as "still being written".
type rewritingProducer struct {
	sentinelProducer
	cache *apptranscoding.SegmentCache
}

func (p *rewritingProducer) EnsureSegmentRange(_ context.Context, fp apptranscoding.DecisionFingerprint, variantIdx, segmentIdx int) error {
	key := apptranscoding.SegmentKey{DecisionHash: fp.Hash(), VariantIdx: variantIdx, SegmentIdx: segmentIdx}
	return os.WriteFile(p.cache.Path(key, fp.UseFMP4)+".tmp", nil, 0o644)
}

func (p *rewritingProducer) WaitBudget() time.Duration { return 50 * time.Millisecond }

// TestHLSVOD_Segment_ReadyAtTheProbeIsServedWithoutAnotherProbe: the handler
// probes the cache once before ensuring the range, and a segment found
// complete there is served. Wait used to probe it again (a stat, a second stat
// and a read of the box headers, on every segment of every viewer), and a
// sibling rewrite started in between turned a complete segment into a 503.
func TestHLSVOD_Segment_ReadyAtTheProbeIsServedWithoutAnotherProbe(t *testing.T) {
	dur, _ := shared.NewDurationMs(60_000)
	item := &domainlibrary.MediaItem{
		ID:        "m1",
		LibraryID: "lib1",
		File:      &domainlibrary.MediaFile{Path: "/test.mkv", Duration: dur},
	}
	h, cacheDir := buildVODHandler(t, &testFakeStreamsGetter{item: item}, allowAll(), testFakeParentalChecker{})
	prod := &rewritingProducer{cache: h.cache}
	h.selector = apptranscoding.NewProducerSelector(prod, prod)

	fp := apptranscoding.DecisionFingerprint{
		MediaID:             "m1",
		SourcePath:          "/test.mkv",
		SubtitleBurnInTrack: -1,
		UseFMP4:             true,
		Profiles:            profilesForTest(),
	}
	segDir := filepath.Join(cacheDir, "vod", "segments", fp.Hash())
	require.NoError(t, os.MkdirAll(segDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(segDir, "stream_0_000.m4s"), []byte{
		0, 0, 0, 0x10, 's', 't', 'y', 'p', 'm', 's', 'd', 'h', 0, 0, 0, 0,
		0, 0, 0, 0x08, 'm', 'o', 'o', 'f',
		0, 0, 0, 0x0C, 'm', 'd', 'a', 't', 0, 0, 0, 0,
	}, 0o644))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/v2/stream_0_000.m4s", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("file", "stream_0_000.m4s")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.serveHTTP(w, req)

	_, err := os.Stat(filepath.Join(segDir, "stream_0_000.m4s.tmp"))
	require.NoError(t, err, "the producer was asked for the range")
	assert.Equal(t, http.StatusOK, w.Code, "a segment complete at the probe is served")
}
