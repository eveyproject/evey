package httpapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
)

// flakyPlan fails its first `fails` reads of every resource with err.
type flakyPlan struct {
	fakeOnDemandPlan
	fails int
	err   error
	reads int
}

func (p *flakyPlan) Resource(ctx context.Context, name string) ([]byte, string, error) {
	p.reads++
	if p.reads <= p.fails {
		return nil, "", p.err
	}
	return p.fakeOnDemandPlan.Resource(ctx, name)
}

func serveSub(t *testing.T, plan apptranscoding.OnDemandPlan, name string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	return serveSubCtx(t, context.Background(), plan, name)
}

func serveSubCtx(t *testing.T, ctx context.Context, plan apptranscoding.OnDemandPlan, name string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	h := &hlsVODServe{libSvc: &testFakeStreamsGetter{}}
	r := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/od_"+name+"?v=x&src=s1.a", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	h.servePlanResourceData(w, r, "m1", plan, nil, name, trace.SpanFromContext(r.Context()))
	return w, buf.String()
}

// A subtitle segment that cannot be read twice is an empty WebVTT window, not
// a 404 a player might treat as fatal; once readable again it is served.
func TestServePlanResourceData_UnreadableSubtitleSegmentIsAnEmptyWindow(t *testing.T) {
	readErr := fmt.Errorf("mp4: read subtitle sample: %w", errors.New("input/output error"))
	vtt := []byte("WEBVTT\n\n00:05:00.000 --> 00:05:02.000\nBonjour\n")

	plan := &flakyPlan{sourceID: "s1", resources: map[string][]byte{"sub2_00050.vtt": vtt}, fails: 2, err: readErr}
	w, logs := serveSub(t, plan, "sub2_00050.vtt")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "WEBVTT\n\n", w.Body.String(), "a valid WebVTT document without a cue")
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	assert.Equal(t, 2, plan.reads, "read once more before giving up")
	assert.Contains(t, logs, "subtitle segment unreadable after a retry")
	assert.Contains(t, logs, "track=2")
	assert.Contains(t, logs, "segment=50")
	assert.Contains(t, logs, "window_start_ms_approx=294000", "names count from 1: sub2_00050 opens at segment 49")

	// A passing error: the retry serves the real cues.
	plan = &flakyPlan{sourceID: "s1", resources: map[string][]byte{"sub2_00050.vtt": vtt}, fails: 1, err: readErr}
	w, logs = serveSub(t, plan, "sub2_00050.vtt")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, string(vtt), w.Body.String())
	assert.NotContains(t, logs, "served without cues")

	// A replaced source keeps its own answer: never an empty window.
	plan = &flakyPlan{sourceID: "s1", resources: map[string][]byte{}, fails: 9, err: fmt.Errorf("x: %w", apptranscoding.ErrSourceChanged)}
	w, _ = serveSub(t, plan, "sub2_00050.vtt")
	assert.NotEqual(t, http.StatusOK, w.Code)

	// A video segment is not a subtitle: its error stays an error, a source
	// that could not be read, which a player retries (502), not a 404.
	plan = &flakyPlan{sourceID: "s1", resources: map[string][]byte{}, fails: 9, err: readErr}
	w, _ = serveSub(t, plan, "seg00050.m4s")
	assert.Equal(t, http.StatusBadGateway, w.Code)
}

// A name the plan refuses is not a read error: a track or a segment out of
// range stays a 404, read once, with no "unreadable" warning.
func TestServePlanResourceData_OutOfRangeSubtitleStaysA404(t *testing.T) {
	plan := &flakyPlan{sourceID: "s1", resources: map[string][]byte{}, fails: 9,
		err: fmt.Errorf("mp4: subtitle rendition 9 out of range (0..2): %w", apptranscoding.ErrNoResource)}
	w, logs := serveSub(t, plan, "sub9_00001.vtt")
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, 1, plan.reads)
	assert.NotContains(t, logs, "unreadable")
}

// A client that left is not a read error: no empty window, no warning.
func TestServePlanResourceData_CancelledSubtitleIsNoWarning(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	plan := &flakyPlan{sourceID: "s1", resources: map[string][]byte{}, fails: 9,
		err: fmt.Errorf("mp4: read subtitle sample: %w", context.Canceled)}
	w, logs := serveSubCtx(t, ctx, plan, "sub2_00050.vtt")
	assert.Zero(t, w.Body.Len(), "nothing is written to a client that left")
	assert.NotContains(t, logs, "unreadable")
	assert.NotContains(t, logs, "refused")
}

// mkvgo 0.48 refuses a segment whose request is already cancelled: a viewer
// that left is no failure, nothing is written, logged or traced. The same
// error while the viewer is still there stays a refused resource.
func TestServePlanResourceData_SegmentOfAViewerThatLeftIsNoWarning(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	plan := &flakyPlan{sourceID: "s1", resources: map[string][]byte{}, fails: 9, err: context.Canceled}
	w, logs := serveSubCtx(t, ctx, plan, "seg00003.m4s")
	assert.Zero(t, w.Body.Len())
	assert.NotContains(t, logs, "refused")
	assert.Contains(t, logs, "level=DEBUG")
	assert.Contains(t, logs, "of a viewer that left")

	for _, err := range []error{context.Canceled, context.DeadlineExceeded} {
		plan = &flakyPlan{sourceID: "s1", resources: map[string][]byte{}, fails: 9, err: err}
		w, logs = serveSubCtx(t, context.Background(), plan, "seg00003.m4s")
		assert.Equal(t, http.StatusBadGateway, w.Code, "%v while the viewer is there", err)
		assert.Contains(t, logs, "level=WARN")
		assert.Contains(t, logs, "on-demand resource refused")
	}
}
