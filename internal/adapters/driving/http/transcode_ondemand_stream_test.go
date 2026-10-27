package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
)

// streamingPlan is a plan whose media segments stream (Bytes nil, bytes
// written on demand) and whose other resources come back whole, like
// mkvgo 0.42's. failAfter > 0 makes the segment write fail after that many
// bytes, the way a source read error does.
type streamingPlan struct {
	fakeOnDemandPlan
	segment   []byte
	failAfter int
	// Touched from the server goroutine, read by the test after the response:
	// the deferred Close can still be running when the client has its bytes.
	mu     sync.Mutex
	opened []string
	closed atomic.Int32
	taken  atomic.Int32 // Bytes calls: each one is a resource counted served
	// strict records every WriteRange span that escapes the resource instead
	// of slicing out of bounds (which would panic the handler and hide it).
	strict bool
	bad    []string
}

func (p *streamingPlan) badRanges() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.bad...)
}

// closedCount waits (briefly) for the handler's deferred Close to have run.
func (p *streamingPlan) closedCount() int32 {
	for range 50 {
		if n := p.closed.Load(); n > 0 {
			return n
		}
		time.Sleep(10 * time.Millisecond)
	}
	return p.closed.Load()
}

func (p *streamingPlan) openedNames() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.opened...)
}

type streamedResource struct {
	p     *streamingPlan
	data  []byte // nil when streaming
	whole []byte
}

func (p *streamingPlan) Open(_ context.Context, name string) (apptranscoding.OnDemandResource, error) {
	p.mu.Lock()
	p.opened = append(p.opened, name)
	p.mu.Unlock()
	if strings.HasPrefix(name, "seg") {
		return &streamedResource{p: p, whole: p.segment}, nil
	}
	if d, ok := p.resources[name]; ok {
		return &streamedResource{p: p, data: d, whole: d}, nil
	}
	return nil, errors.New("fake: no resource " + name)
}

func (r *streamedResource) ContentType() string { return "video/iso.segment" }
func (r *streamedResource) Size() int64         { return int64(len(r.whole)) }
func (r *streamedResource) ETag() string {
	if r.data != nil {
		return `"strong"`
	}
	return `W/"weak-seg"`
}
func (r *streamedResource) Bytes() []byte { r.p.taken.Add(1); return r.data }
func (r *streamedResource) Whole() bool   { return r.data != nil }
func (r *streamedResource) Close() error  { r.p.closed.Add(1); return nil }
func (r *streamedResource) WriteRange(_ context.Context, w io.Writer, off, n int64) (int64, error) {
	if off < 0 || n < 0 || off+n > int64(len(r.whole)) {
		r.p.mu.Lock()
		r.p.bad = append(r.p.bad, fmt.Sprintf("off=%d n=%d size=%d", off, n, len(r.whole)))
		r.p.mu.Unlock()
		if r.p.strict {
			return 0, errors.New("span outside the resource")
		}
	}
	chunk := r.whole[off : off+n]
	if r.p.failAfter > 0 && len(chunk) > r.p.failAfter {
		m, _ := w.Write(chunk[:r.p.failAfter])
		return int64(m), errors.New("source read failed")
	}
	m, err := w.Write(chunk)
	return int64(m), err
}

func newStreamingServer(t *testing.T, plan *streamingPlan) *httptest.Server {
	t.Helper()
	h := &hlsVODServe{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		h.servePlanResourceData(w, r, "media-1", plan, nil, name, trace.SpanFromContext(r.Context()))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestServePlanResource_StreamsASegmentWithExactLengthAndWeakETag(t *testing.T) {
	seg := []byte(strings.Repeat("s", 10_000))
	plan := &streamingPlan{segment: seg}
	srv := newStreamingServer(t, plan)

	res, err := http.Get(srv.URL + "/seg00001.m4s")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK || string(body) != string(seg) {
		t.Fatalf("status %d, %d bytes; want 200 and the whole segment", res.StatusCode, len(body))
	}
	if got := res.Header.Get("Content-Length"); got != "10000" {
		t.Fatalf("Content-Length = %q, want 10000", got)
	}
	if got := res.Header.Get("ETag"); got != `W/"weak-seg"` {
		t.Fatalf("ETag = %q, want the weak tag", got)
	}
	if got := res.Header.Get("X-Evey-Served-By"); got != "mkvgo" {
		t.Fatalf("X-Evey-Served-By = %q", got)
	}
	if got := res.Header.Get("Cache-Control"); !strings.Contains(got, "immutable") {
		t.Fatalf("Cache-Control = %q, want immutable", got)
	}
	if got := res.Header.Get("Accept-Ranges"); got != "bytes" {
		t.Fatalf("Accept-Ranges = %q", got)
	}
	if n := plan.closedCount(); n != 1 {
		t.Fatalf("handle closed %d times, want 1", n)
	}
}

func TestServePlanResource_StreamedRangeAndHead(t *testing.T) {
	seg := []byte("0123456789")
	srv := newStreamingServer(t, &streamingPlan{segment: seg})

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/seg00001.m4s", nil)
	req.Header.Set("Range", "bytes=2-5")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusPartialContent || string(body) != "2345" {
		t.Fatalf("range: status %d body %q", res.StatusCode, body)
	}
	if got := res.Header.Get("Content-Range"); got != "bytes 2-5/10" {
		t.Fatalf("Content-Range = %q", got)
	}

	req, _ = http.NewRequest(http.MethodHead, srv.URL+"/seg00001.m4s", nil)
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK || len(body) != 0 || res.Header.Get("Content-Length") != "10" {
		t.Fatalf("HEAD: status %d, body %d bytes, Content-Length %q", res.StatusCode, len(body), res.Header.Get("Content-Length"))
	}

	req, _ = http.NewRequest(http.MethodGet, srv.URL+"/seg00001.m4s", nil)
	req.Header.Set("Range", "bytes=50-60")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("out-of-range: status %d, want 416", res.StatusCode)
	}
}

func TestServePlanResource_StreamedIfNoneMatchIs304(t *testing.T) {
	srv := newStreamingServer(t, &streamingPlan{segment: []byte("abc")})
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/seg00001.m4s", nil)
	req.Header.Set("If-None-Match", `W/"weak-seg"`)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNotModified {
		t.Fatalf("status %d, want 304", res.StatusCode)
	}
}

// A source read error after the status line must not end as a clean short
// body: the client has to see a broken transfer.
func TestServePlanResource_StreamedWriteErrorCutsTheConnection(t *testing.T) {
	seg := []byte(strings.Repeat("x", 100_000))
	srv := newStreamingServer(t, &streamingPlan{segment: seg, failAfter: 1000})
	res, err := http.Get(srv.URL + "/seg00001.m4s")
	if err != nil {
		// The 1000 bytes written never left net/http's buffer before the
		// abort, so the client saw the connection drop before any status line
		// - a broken transfer, which is the point.
		return
	}
	body, readErr := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if readErr == nil {
		t.Fatalf("read %d of %d bytes with no error: a truncated segment passed as whole", len(body), len(seg))
	}
}

// Whatever Open returns whole goes down the buffered path unchanged: the VTT
// is sanitised, the cache header set, and the handle closed.
func TestServePlanResource_BufferedHandleKeepsTheOldPath(t *testing.T) {
	plan := &streamingPlan{resources: map[string][]byte{
		"sub1_00001.vtt": []byte("WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nhello\n"),
	}}
	srv := newStreamingServer(t, plan)
	res, err := http.Get(srv.URL + "/sub1_00001.vtt")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK || !strings.HasPrefix(string(body), "WEBVTT") {
		t.Fatalf("status %d body %q", res.StatusCode, body)
	}
	if got := res.Header.Get("ETag"); got != "" {
		t.Fatalf("buffered path grew an ETag %q", got)
	}
	if n := plan.closedCount(); n != 1 {
		t.Fatalf("handle closed %d times, want 1", n)
	}
}

// A HEAD of a resource Open holds whole is answered from its size, its bytes
// never taken (taking them counts them served); a GET takes them once.
func TestServePlanResource_HeadOfAWholeResourceTakesNoBytes(t *testing.T) {
	plan := &streamingPlan{resources: map[string][]byte{"init_v.mp4": []byte("0123456789")}}
	srv := newStreamingServer(t, plan)
	req, _ := http.NewRequest(http.MethodHead, srv.URL+"/init_v.mp4", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Length") != "10" || res.Header.Get("ETag") != `"strong"` {
		t.Fatalf("HEAD: status %d, Content-Length %q, ETag %q", res.StatusCode, res.Header.Get("Content-Length"), res.Header.Get("ETag"))
	}
	if n := plan.closedCount(); n != 1 {
		t.Fatalf("handle closed %d times, want 1", n)
	}
	if n := plan.taken.Load(); n != 0 {
		t.Fatalf("HEAD took the bytes %d times, want 0", n)
	}
	res, err = http.Get(srv.URL + "/init_v.mp4")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if string(body) != "0123456789" {
		t.Fatalf("GET body %q", body)
	}
	if n := plan.taken.Load(); n != 1 {
		t.Fatalf("GET took the bytes %d times, want 1", n)
	}
}

// Playlists are rewritten before they go out, so they never take the handle.
func TestServePlanResource_PlaylistsStayOnResource(t *testing.T) {
	plan := &streamingPlan{resources: map[string][]byte{
		"playlist.m3u8": []byte("#EXTM3U\nseg00001.m4s\n"),
	}}
	srv := newStreamingServer(t, plan)
	res, err := http.Get(srv.URL + "/playlist.m3u8")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if opened := plan.openedNames(); len(opened) != 0 {
		t.Fatalf("playlist went through Open: %v", opened)
	}
	if !strings.Contains(string(body), "seg00001.m4s?v=") {
		t.Fatalf("playlist not rebaked: %q", body)
	}
}

func TestSingleByteRange(t *testing.T) {
	const size = 100
	cases := []struct {
		hdr      string
		start, n int64
		status   int
		ok       bool
	}{
		// no range, open ranges
		{"", 0, 100, 200, true},
		{"bytes=0-", 0, 100, 206, true},
		{"bytes=-", 0, 100, 200, true},
		{"bytes=10-19", 10, 10, 206, true},
		{"bytes=99-99", 99, 1, 206, true},
		// past the end: last clamps, first is unsatisfiable
		{"bytes=90-200", 90, 10, 206, true},
		{"bytes=100-", 0, 0, 0, false},
		{"bytes=100-200", 0, 0, 0, false},
		// suffix: clamped to the file, an empty suffix is unsatisfiable
		{"bytes=-10", 90, 10, 206, true},
		{"bytes=-1000", 0, 100, 206, true},
		{"bytes=-0", 0, 0, 0, false},
		// negative numbers are not a range: ignored, served whole
		{"bytes=-5-10", 0, 100, 200, true},
		{"bytes=5--10", 0, 100, 200, true},
		{"bytes=+5-10", 0, 100, 200, true},
		// overflow: a first byte beyond int64 is past the end, a last byte beyond int64 clamps
		{"bytes=99999999999999999999-", 0, 0, 0, false},
		{"bytes=0-99999999999999999999", 0, 100, 206, true},
		{"bytes=9223372036854775807-9223372036854775807", 0, 0, 0, false},
		{"bytes=-99999999999999999999", 0, 100, 206, true},
		// multiple ranges and malformed syntax: served whole
		{"bytes=0-9,20-29", 0, 100, 200, true},
		{"bytes=5-2", 0, 100, 200, true},
		{"bytes=abc", 0, 100, 200, true},
		{"bytes=a-b", 0, 100, 200, true},
		{"bytes=0x10-", 0, 100, 200, true},
		{"items=0-10", 0, 100, 200, true},
	}
	for _, c := range cases {
		start, n, status, ok := singleByteRange(c.hdr, size)
		if start != c.start || n != c.n || status != c.status || ok != c.ok {
			t.Errorf("%q: got (%d,%d,%d,%v) want (%d,%d,%d,%v)", c.hdr, start, n, status, ok, c.start, c.n, c.status, c.ok)
		}
		if ok && (start < 0 || n < 0 || start+n > size) {
			t.Errorf("%q: span (%d,%d) escapes [0,%d]", c.hdr, start, n, size)
		}
	}
}

// Whatever the Range header says, the handle is only ever asked for a span
// inside the resource: every accepted request reaches WriteRange with
// 0 <= off, 0 <= n, off+n <= size; a refused one never reaches it.
func TestServePlanResource_RangeIsValidatedBeforeWriteRange(t *testing.T) {
	seg := []byte(strings.Repeat("r", 1000))
	plan := &streamingPlan{segment: seg, strict: true}
	srv := newStreamingServer(t, plan)
	for _, hdr := range []string{"", "bytes=0-", "bytes=10-19", "bytes=990-5000", "bytes=-10", "bytes=-100000", "bytes=1000-", "bytes=5000-6000", "bytes=-0", "bytes=-5-10", "bytes=99999999999999999999-", "bytes=0-99999999999999999999", "bytes=0-9,20-29", "bytes=5-2", "bytes=abc"} {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/seg00001.m4s", nil)
		if hdr != "" {
			req.Header.Set("Range", hdr)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%q: %v", hdr, err)
		}
		body, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		switch res.StatusCode {
		case http.StatusOK:
			if len(body) != len(seg) {
				t.Errorf("%q: 200 with %d bytes, want the whole %d", hdr, len(body), len(seg))
			}
		case http.StatusPartialContent:
			if len(body) == 0 || len(body) > len(seg) {
				t.Errorf("%q: 206 with %d bytes", hdr, len(body))
			}
		case http.StatusRequestedRangeNotSatisfiable:
			if got := res.Header.Get("Content-Range"); got != "bytes */1000" {
				t.Errorf("%q: 416 without the size: Content-Range=%q", hdr, got)
			}
		default:
			t.Errorf("%q: status %d", hdr, res.StatusCode)
		}
	}
	if bad := plan.badRanges(); len(bad) != 0 {
		t.Fatalf("WriteRange reached with spans outside the resource: %v", bad)
	}
}
