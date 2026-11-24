package httpapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	mkvgoadapter "github.com/eveyproject/evey/internal/adapters/driven/mkvgo"
	"github.com/eveyproject/evey/internal/adapters/driven/prober"
	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	domainplayback "github.com/eveyproject/evey/internal/domain/playback"
)

// TestServedWitnessPackagerIntegration serves a two-audio-track film through
// the real mkvgo packager, under the served witness, and checks that what the
// session is told is what went out: each audio rendition is the source track
// the file numbers it (MP3 first, AAC second, as muxed), its codec read from
// the init the packager produced, and a segment fetched without its init
// (the browser kept the init) is still described. Opt-in:
//
//	EVEY_TRANSCODE_ITEST=1 go test ./internal/adapters/driving/http/ -run ServedWitnessPackagerIntegration -v
func TestServedWitnessPackagerIntegration(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (use `make transcode-itest`)")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not in PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	src := filepath.Join(t.TempDir(), "movie.mkv")
	cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
		"-f", "lavfi", "-i", "sine=frequency=880:sample_rate=48000",
		"-t", "12", "-map", "0", "-map", "1", "-map", "2",
		"-c:v", "libx264", "-g", "48", "-keyint_min", "48", "-pix_fmt", "yuv420p",
		"-c:a:0", "libmp3lame", "-c:a:1", "aac",
		src)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg source generation failed: %v\n%s", err, out)
	}
	// The streams as the scan stores them: the production prober, whose mkvgo
	// path carries the number the file gives each track.
	probed, err := prober.New(nil, nil, nil).ProbeStreams(ctx, src)
	require.NoError(t, err)
	streams := probed.Streams

	opts := apptranscoding.OnDemandOptions{SegmentMs: onDemandSegmentMs, URLPrefix: onDemandPrefix}
	pkg := mkvgoadapter.NewOnDemandPackager()
	defer pkg.Close()
	h := &hlsVODServe{onDemand: pkg}

	rec := &legRecorder{}
	sw := &servedWitness{record: rec.record}
	handler := sw.wrap(func(w http.ResponseWriter, r *http.Request) {
		plan, err := pkg.Plan(r.Context(), src, opts)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		name := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/"), onDemandPrefix)
		stampPackagerResource(r, "m1", plan, nil, name, streams)
		h.servePlanResourceData(w, r, "m1", plan, nil, name, trace.SpanFromContext(r.Context()))
	})
	get := func(name string) {
		t.Helper()
		req := withHTTPClaims(httptest.NewRequest(http.MethodGet, "/"+onDemandPrefix+name, nil), testClaims(false))
		w := httptest.NewRecorder()
		handler(w, req)
		require.Equal(t, http.StatusOK, w.Code, "%s: %s", name, w.Body.String())
		_, _ = io.Copy(io.Discard, w.Body)
	}
	last := func(leg domainplayback.Leg) domainplayback.ServedLeg {
		t.Helper()
		legs := rec.byLeg(leg)
		require.NotEmpty(t, legs)
		return legs[len(legs)-1]
	}

	get("init.mp4")
	video := last(domainplayback.LegVideo)
	assert.Equal(t, domainplayback.EngineMkvgo, video.Engine)
	assert.True(t, video.Copy)
	assert.Equal(t, "h264", video.Codec)
	assert.Equal(t, 180, video.Height)
	assert.Empty(t, rec.byLeg(domainplayback.LegAudio), "the primary rendition carries the video only")

	get("init_a2.mp4")
	aac := last(domainplayback.LegAudio)
	assert.Equal(t, "aac", aac.Codec)
	assert.Equal(t, 1, aac.Track, "rendition 2 is the file's second audio track")

	// A fresh witness: the init of rendition 1 was never seen going out.
	sw.memo = initMemo{}
	get("seg_a1_00001.m4s")
	mp3 := last(domainplayback.LegAudio)
	assert.Equal(t, "mp3", mp3.Codec)
	assert.Equal(t, 0, mp3.Track)
	assert.True(t, mp3.Copy)
}
