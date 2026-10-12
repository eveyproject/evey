package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	domaintranscoding "github.com/eveyproject/evey/internal/domain/transcoding"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/reduceddl"
)

// fakeReducedTranscoder stands in for the hour of ffmpeg: it writes n slice
// files plus the segment muxer's list, exactly as the real producer would, and
// records the spec it was handed (the resume assertions read it back).
type fakeReducedTranscoder struct {
	slices  int
	err     error
	calls   atomic.Int32
	lastMu  chan struct{} // buffered(1), guards lastSpec without a mutex ceremony
	lastVal reduceddl.Spec
}

func newFakeReducedTranscoder(slices int) *fakeReducedTranscoder {
	f := &fakeReducedTranscoder{slices: slices, lastMu: make(chan struct{}, 1)}
	f.lastMu <- struct{}{}
	return f
}

func (f *fakeReducedTranscoder) TranscodeReducedSlices(_ context.Context, _, dir string, spec reduceddl.Spec, _ domaintranscoding.PicturePlan, _, _ int64, onProgress func(int)) error {
	f.calls.Add(1)
	<-f.lastMu
	f.lastVal = spec
	f.lastMu <- struct{}{}
	if onProgress != nil {
		onProgress(420)
	}
	if f.err != nil {
		return f.err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var list strings.Builder
	for i := spec.StartSlice; i < spec.StartSlice+f.slices; i++ {
		name := reduceddl.SliceName(i)
		if err := os.WriteFile(filepath.Join(dir, name), fmt.Appendf(nil, "slice-%d-bytes", i), 0o644); err != nil {
			return err
		}
		start := float64(i * spec.SliceSec)
		fmt.Fprintf(&list, "%s,%.6f,%.6f\n", name, start, start+float64(spec.SliceSec))
	}
	return os.WriteFile(filepath.Join(dir, reduceddl.ListName(spec.StartSlice)), []byte(list.String()), 0o644)
}

func (f *fakeReducedTranscoder) spec() reduceddl.Spec {
	<-f.lastMu
	defer func() { f.lastMu <- struct{}{} }()
	return f.lastVal
}

// fakeReducedStreams answers with one 4K video track (so a reduced variant is
// warranted) unless told otherwise.
type fakeReducedStreams struct{ w, h int }

func (f *fakeReducedStreams) GetMediaStreams(context.Context, string) ([]*domainlibrary.MediaStream, error) {
	w, h := f.w, f.h
	return []*domainlibrary.MediaStream{
		{StreamType: domainlibrary.Video, Codec: "hevc", Width: &w, Height: &h},
		{StreamType: domainlibrary.Audio, Codec: "eac3"},
	}, nil
}

// reducedTestItem is a 30-minute, 8 GiB film: three slices at the production
// slice length, and comfortably over any sane offer threshold.
const (
	reducedTestDurationMs = 30 * 60 * 1000
	reducedTestSize       = 8 << 30
)

func reducedTestItem() *domainlibrary.MediaItem {
	dur, _ := shared.NewDurationMs(reducedTestDurationMs)
	return &domainlibrary.MediaItem{
		ID:        "m1",
		LibraryID: "lib1",
		Title:     "Big Film",
		File: &domainlibrary.MediaFile{
			Path:        "/tmp/fake.mkv",
			Size:        reducedTestSize,
			Duration:    dur,
			Fingerprint: "fp1",
		},
	}
}

func newTestReducedServe(t *testing.T, tr ReducedTranscoder, access LibraryAccessChecker, streams StreamsByMediaGetter) *reducedServe {
	t.Helper()
	if streams == nil {
		streams = &fakeReducedStreams{w: 3840, h: 2160}
	}
	return newReducedServe(&fakeMediaItemGetter{item: reducedTestItem()}, streams, access,
		testFakeParentalChecker{}, nil, tr, t.TempDir(), "http://localhost", 4<<30)
}

func doReducedRequest(h http.HandlerFunc, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h(w, req)
	return w
}

func doReducedSlice(h *reducedServe, idx int) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/media/m1/reduced-download/slice/%d", idx), nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("n", fmt.Sprint(idx))
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.handleSlice(w, req)
	return w
}

func decodeReduced(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

func waitReducedStatus(t *testing.T, h *reducedServe, want string) map[string]any {
	t.Helper()
	var last map[string]any
	require.Eventually(t, func() bool {
		last = decodeReduced(t, doReducedRequest(h.handleStatus, http.MethodGet, "/api/v1/media/m1/reduced-download/status"))
		return last["status"] == want
	}, 2*time.Second, 10*time.Millisecond, "status never reached %q (last: %v)", want, last)
	return last
}

// The plan endpoint decides whether the app even shows the choice, and it must
// do so without starting an hour of NAS work.
func TestReducedDownload_PlanOffersOnlyWhenItIsWorthIt(t *testing.T) {
	tr := newFakeReducedTranscoder(3)
	h := newTestReducedServe(t, tr, allowAll(), nil)

	body := decodeReduced(t, doReducedRequest(h.handlePlan, http.MethodGet, "/api/v1/media/m1/reduced-download/plan"))
	assert.Equal(t, true, body["available"])
	assert.Equal(t, true, body["offer"], "8 GiB is over the 4 GiB threshold")
	assert.EqualValues(t, 1280, body["width"], "3840x2160 delivered at height 720")
	assert.EqualValues(t, reduceddl.EstimatedBytes(reducedTestDurationMs), body["estimatedBytes"])
	assert.EqualValues(t, 3, body["slices"])
	assert.EqualValues(t, reducedTestDurationMs, body["durationMs"])
	assert.EqualValues(t, 0, tr.calls.Load(), "planning must never start a production")

	// A source already at or below the target: offering a "lighter" copy that
	// costs an hour and saves nothing is worse than not offering.
	small := newTestReducedServe(t, tr, allowAll(), &fakeReducedStreams{w: 1280, h: 720})
	body = decodeReduced(t, doReducedRequest(small.handlePlan, http.MethodGet, "/api/v1/media/m1/reduced-download/plan"))
	assert.Equal(t, false, body["available"])
	assert.Equal(t, false, body["offer"])

	// Over the bar for a reduced copy, but under the operator's threshold:
	// available (the phone may still ask, e.g. it is out of space) but not
	// offered on the server's own initiative.
	quiet := newReducedServe(&fakeMediaItemGetter{item: reducedTestItem()}, &fakeReducedStreams{w: 3840, h: 2160},
		allowAll(), testFakeParentalChecker{}, nil, tr, t.TempDir(), "http://localhost", 32<<30)
	body = decodeReduced(t, doReducedRequest(quiet.handlePlan, http.MethodGet, "/api/v1/media/m1/reduced-download/plan"))
	assert.Equal(t, true, body["available"])
	assert.Equal(t, false, body["offer"])
}

// The core loop: prepare, collect the slices in order, and watch the server's
// footprint shrink behind the phone.
func TestReducedDownload_SlicesAreServedThenReleased(t *testing.T) {
	tr := newFakeReducedTranscoder(3)
	h := newTestReducedServe(t, tr, allowAll(), nil)

	// Nothing produced yet: asking for slice 0 is a "come back later", not an
	// error the client should give up on.
	w := doReducedSlice(h, 0)
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Equal(t, "absent", decodeReduced(t, w)["status"])

	w = doReducedRequest(h.handlePrepare, http.MethodPost, "/api/v1/media/m1/reduced-download")
	assert.Equal(t, http.StatusAccepted, w.Code)
	ready := waitReducedStatus(t, h, "ready")
	assert.EqualValues(t, 3, ready["slicesReady"])
	assert.EqualValues(t, 3, ready["slices"])
	assert.EqualValues(t, 1, tr.calls.Load())

	jobDir := filepath.Join(h.cacheDir, reducedJobDirName("m1", "fp1", "p:p1"))
	for i := range 3 {
		w = doReducedSlice(h, i)
		require.Equal(t, http.StatusOK, w.Code, "slice %d", i)
		assert.Equal(t, fmt.Sprintf("slice-%d-bytes", i), w.Body.String())
		assert.Contains(t, w.Header().Get("Content-Disposition"), "attachment")

		// Fetching slice i is the ack for everything before it. That release
		// is what keeps the server from holding a second copy of the film.
		for j := range i {
			_, err := os.Stat(filepath.Join(jobDir, reduceddl.SliceName(j)))
			assert.True(t, os.IsNotExist(err), "slice %d should have been released once %d was fetched", j, i)
		}
	}

	// Re-asking for a released slice is 410, distinguishable from 409 "not
	// yet" - a client that cannot tell them apart waits forever.
	w = doReducedSlice(h, 0)
	assert.Equal(t, http.StatusGone, w.Code)

	// And past the end is still "not ready", never a 200 of nothing.
	w = doReducedSlice(h, 3)
	assert.Equal(t, http.StatusConflict, w.Code)
}

// A production the phone can start collecting before it finishes IS the
// feature: waiting for the whole film would reintroduce the hour of exposure
// the slicing exists to remove.
func TestReducedDownload_SlicesAreCollectableWhileTheProductionRuns(t *testing.T) {
	tr := newFakeReducedTranscoder(3)
	h := newTestReducedServe(t, tr, allowAll(), nil)
	jobDir := filepath.Join(h.cacheDir, reducedJobDirName("m1", "fp1", "p:p1"))

	// Two slices on disk, no producer registered and the film is longer than
	// that: the status must not claim "ready", and slice 0 must still serve.
	require.NoError(t, os.MkdirAll(jobDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(jobDir, reduceddl.SliceName(0)), []byte("s0"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(jobDir, reduceddl.SliceName(1)), []byte("s1"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(jobDir, reduceddl.ListName(0)),
		[]byte("slice-00000.mkv,0.000000,601.500000\nslice-00001.mkv,601.500000,1203.200000\n"), 0o644))

	body := decodeReduced(t, doReducedRequest(h.handleStatus, http.MethodGet, "/api/v1/media/m1/reduced-download/status"))
	assert.Equal(t, "interrupted", body["status"])
	assert.EqualValues(t, 2, body["slicesReady"])
	assert.Equal(t, http.StatusOK, doReducedSlice(h, 0).Code)
	assert.Equal(t, http.StatusConflict, doReducedSlice(h, 2).Code)

	// Resuming picks up at slice 2, seeking to the END the muxer recorded -
	// not 2*600. The slices ran long, and the arithmetic guess would drop
	// three seconds of film on the floor.
	w := doReducedRequest(h.handlePrepare, http.MethodPost, "/api/v1/media/m1/reduced-download")
	assert.Equal(t, http.StatusAccepted, w.Code)
	waitReducedStatus(t, h, "ready")
	spec := tr.spec()
	assert.Equal(t, 2, spec.StartSlice)
	assert.InDelta(t, 1203.2, spec.StartSec, 0.001)
}

// A source that is already small enough must cost nothing and say so, rather
// than looking like a failure the client should retry.
func TestReducedDownload_UnnecessaryForAnAlreadySmallSource(t *testing.T) {
	tr := newFakeReducedTranscoder(3)
	h := newTestReducedServe(t, tr, allowAll(), &fakeReducedStreams{w: 1280, h: 720})

	w := doReducedRequest(h.handlePrepare, http.MethodPost, "/api/v1/media/m1/reduced-download")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "unnecessary", decodeReduced(t, w)["status"])
	assert.EqualValues(t, 0, tr.calls.Load())
}

func TestReducedDownload_FailureIsReportedAndRetryable(t *testing.T) {
	tr := newFakeReducedTranscoder(3)
	tr.err = errors.New("disk full")
	h := newTestReducedServe(t, tr, allowAll(), nil)

	w := doReducedRequest(h.handlePrepare, http.MethodPost, "/api/v1/media/m1/reduced-download")
	assert.Equal(t, http.StatusAccepted, w.Code)
	failed := waitReducedStatus(t, h, "failed")
	assert.Contains(t, failed["error"], "disk full")

	tr.err = nil
	w = doReducedRequest(h.handlePrepare, http.MethodPost, "/api/v1/media/m1/reduced-download")
	assert.Equal(t, http.StatusAccepted, w.Code)
	waitReducedStatus(t, h, "ready")
	assert.EqualValues(t, 2, tr.calls.Load())
}

// panickingReducedTranscoder stands in for an encoder that panics mid-job.
type panickingReducedTranscoder struct{}

func (panickingReducedTranscoder) TranscodeReducedSlices(context.Context, string, string, reduceddl.Spec, domaintranscoding.PicturePlan, int64, int64, func(int)) error {
	panic("encoder blew up")
}

// The production runs on a goroutine detached from the request: a panic there
// took the whole server down. It must end as a failed job instead.
func TestReducedDownload_PanicEndsAsAFailedJob(t *testing.T) {
	h := newTestReducedServe(t, panickingReducedTranscoder{}, allowAll(), nil)

	w := doReducedRequest(h.handlePrepare, http.MethodPost, "/api/v1/media/m1/reduced-download")
	assert.Equal(t, http.StatusAccepted, w.Code)
	failed := waitReducedStatus(t, h, "failed")
	assert.Contains(t, failed["error"], "encoder blew up")
}

// The phone saying "done" must actually free the disk, and must not be able to
// pull the floor out from under a running ffmpeg.
func TestReducedDownload_DiscardFreesTheJobButNotUnderARunningProduction(t *testing.T) {
	tr := newFakeReducedTranscoder(3)
	h := newTestReducedServe(t, tr, allowAll(), nil)
	jobDir := filepath.Join(h.cacheDir, reducedJobDirName("m1", "fp1", "p:p1"))

	doReducedRequest(h.handlePrepare, http.MethodPost, "/api/v1/media/m1/reduced-download")
	waitReducedStatus(t, h, "ready")
	require.DirExists(t, jobDir)

	w := doReducedRequest(h.handleDiscard, http.MethodDelete, "/api/v1/media/m1/reduced-download")
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.NoDirExists(t, jobDir)

	// While a production is running, discarding is refused: removing the dir
	// would leave ffmpeg writing into unlinked files for the rest of the hour.
	h.mu.Lock()
	h.jobs[jobDir] = &reducedJob{running: true}
	h.mu.Unlock()
	require.NoError(t, os.MkdirAll(jobDir, 0o755))
	w = doReducedRequest(h.handleDiscard, http.MethodDelete, "/api/v1/media/m1/reduced-download")
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.DirExists(t, jobDir)
}

// A phone that vanishes mid-download must not pin the tail of a film forever.
func TestReducedDownload_IdleSweepReclaimsAbandonedJobs(t *testing.T) {
	tr := newFakeReducedTranscoder(3)
	h := newTestReducedServe(t, tr, allowAll(), nil)

	abandoned := filepath.Join(h.cacheDir, "reduced-v1-abandoned")
	fresh := filepath.Join(h.cacheDir, "reduced-v1-fresh")
	running := filepath.Join(h.cacheDir, "reduced-v1-running")
	for _, d := range []string{abandoned, fresh, running} {
		require.NoError(t, os.MkdirAll(d, 0o755))
	}
	old := time.Now().Add(-reducedIdleTTL - time.Hour)
	require.NoError(t, os.Chtimes(abandoned, old, old))
	require.NoError(t, os.Chtimes(running, old, old))
	h.mu.Lock()
	h.jobs[running] = &reducedJob{running: true}
	h.mu.Unlock()

	h.sweepIdle()

	assert.NoDirExists(t, abandoned)
	assert.DirExists(t, fresh, "a job touched recently is still being collected")
	assert.DirExists(t, running, "ffmpeg is writing in there")
}

func TestReducedDownload_GuardsMirrorTheRawDownload(t *testing.T) {
	tr := newFakeReducedTranscoder(3)

	// Library access refused: no production, and 404 rather than 403 so a
	// denied library stays indistinguishable from a missing item.
	denied := newTestReducedServe(t, tr, &fakeLibraryAccessChecker{allowed: false}, nil)
	for name, h := range map[string]http.HandlerFunc{
		"plan":    denied.handlePlan,
		"prepare": denied.handlePrepare,
		"status":  denied.handleStatus,
		"discard": denied.handleDiscard,
	} {
		w := doReducedRequest(h, http.MethodPost, "/api/v1/media/m1/reduced-download")
		assert.Equal(t, http.StatusNotFound, w.Code, "%s must refuse a denied library", name)
	}
	assert.Equal(t, http.StatusNotFound, doReducedSlice(denied, 0).Code)
	assert.EqualValues(t, 0, tr.calls.Load())

	// A stream token scoped to another media id is an IDOR attempt - on every
	// verb, including the one that hands out bytes.
	h := newTestReducedServe(t, tr, allowAll(), nil)
	for name, handler := range map[string]http.HandlerFunc{
		"prepare": h.handlePrepare,
		"slice":   h.handleSlice,
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/reduced-download", nil)
		req.SetPathValue("id", "m1")
		req.SetPathValue("n", "0")
		req = withHTTPClaims(req, testClaims(false))
		req = req.WithContext(context.WithValue(req.Context(), streamTokenCtxKey, &shared.StreamToken{MediaItemID: "OTHER"}))
		rec := httptest.NewRecorder()
		handler(rec, req)
		assert.Equal(t, http.StatusForbidden, rec.Code, "%s must refuse a token scoped elsewhere", name)
	}

	// A title the profile's parental rules hide is a 404 on every verb and is
	// never produced, library access notwithstanding.
	blocked := newTestReducedServe(t, tr, allowAll(), nil)
	blocked.parental = testFakeParentalChecker{block: true}
	for name, handler := range map[string]http.HandlerFunc{
		"plan":    blocked.handlePlan,
		"prepare": blocked.handlePrepare,
		"status":  blocked.handleStatus,
	} {
		w := doReducedRequest(handler, http.MethodPost, "/api/v1/media/m1/reduced-download")
		assert.Equal(t, http.StatusNotFound, w.Code, "%s must hide a parental-blocked title", name)
	}
	assert.Equal(t, http.StatusNotFound, doReducedSlice(blocked, 0).Code)
	assert.EqualValues(t, 0, tr.calls.Load())
	assert.EqualValues(t, 0, tr.calls.Load())
}

// A slice index is attacker-controlled path input. It must never reach
// filepath.Join as anything but a number.
func TestReducedDownload_RejectsABadSliceIndex(t *testing.T) {
	h := newTestReducedServe(t, newFakeReducedTranscoder(3), allowAll(), nil)
	for _, bad := range []string{"-1", "abc", "../../etc/passwd", ""} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/reduced-download/slice/x", nil)
		req.SetPathValue("id", "m1")
		req.SetPathValue("n", bad)
		req = withHTTPClaims(req, testClaims(false))
		w := httptest.NewRecorder()
		h.handleSlice(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code, "slice index %q", bad)
	}
}

// Two profiles fetching the same title get their own job: the slices one
// phone acks (and the discard it sends) must never take the other's bytes away.
func TestReducedDownload_JobsAreKeyedPerConsumer(t *testing.T) {
	tr := newFakeReducedTranscoder(3)
	h := newTestReducedServe(t, tr, allowAll(), nil)
	as := func(profile string, fn http.HandlerFunc, method, path string, pathVals ...string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req.SetPathValue("id", "m1")
		for i := 0; i+1 < len(pathVals); i += 2 {
			req.SetPathValue(pathVals[i], pathVals[i+1])
		}
		req = withHTTPClaims(req, &domainuser.Claims{UserID: "u1", ProfileID: profile})
		w := httptest.NewRecorder()
		fn(w, req)
		return w
	}

	as("p1", h.handlePrepare, http.MethodPost, "/api/v1/media/m1/reduced-download")
	as("p2", h.handlePrepare, http.MethodPost, "/api/v1/media/m1/reduced-download")
	require.Eventually(t, func() bool {
		a := decodeReduced(t, as("p1", h.handleStatus, http.MethodGet, "/api/v1/media/m1/reduced-download/status"))
		b := decodeReduced(t, as("p2", h.handleStatus, http.MethodGet, "/api/v1/media/m1/reduced-download/status"))
		return a["status"] == "ready" && b["status"] == "ready"
	}, 2*time.Second, 10*time.Millisecond)

	// p2 acks slice 0 by fetching slice 1; p1 must still be able to fetch slice 0.
	require.Equal(t, http.StatusOK, as("p2", h.handleSlice, http.MethodGet, "/api/v1/media/m1/reduced-download/slice/1", "n", "1").Code)
	assert.Equal(t, http.StatusOK, as("p1", h.handleSlice, http.MethodGet, "/api/v1/media/m1/reduced-download/slice/0", "n", "0").Code,
		"p2 consuming its copy released p1's slice")

	// p1 is done; p2's job survives.
	require.Equal(t, http.StatusNoContent, as("p1", h.handleDiscard, http.MethodDelete, "/api/v1/media/m1/reduced-download").Code)
	assert.Equal(t, http.StatusOK, as("p2", h.handleSlice, http.MethodGet, "/api/v1/media/m1/reduced-download/slice/2", "n", "2").Code,
		"p1 discarding its job deleted p2's")
}
