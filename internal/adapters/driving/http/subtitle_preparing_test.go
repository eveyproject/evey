package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
)

// A sparse "film": the size drives the estimate, and the bytes are never read
// on the request path (the detached extraction fails on them in milliseconds,
// which is what the failure memo test wants).
func sparseMKV(t *testing.T, size int64) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "film.mkv")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(size); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestEstimateSubtitleExtraction_WalkScalesWithSize_IndexWithBlocks(t *testing.T) {
	small := sparseMKV(t, 100<<20)
	big := sparseMKV(t, 20<<30)
	if est := estimateSubtitleExtraction(small, 0, false); est > subtitlePreparingThreshold {
		t.Errorf("a 100 MB file must be under the threshold, got %s", est)
	}
	if est := estimateSubtitleExtraction(big, 0, false); est <= subtitlePreparingThreshold {
		t.Errorf("a 20 GB walk must be over the threshold, got %s", est)
	}
	// Indexed: 3172 cues (the measured French track of the 85 GB remux) is
	// ~10 s - served inline; 10 000 blocks is over.
	if est := estimateSubtitleExtraction(big, 3172, true); est > subtitlePreparingThreshold {
		t.Errorf("3172 indexed blocks must be under the threshold, got %s", est)
	}
	if est := estimateSubtitleExtraction(big, 10_000, true); est <= subtitlePreparingThreshold {
		t.Errorf("10 000 indexed blocks must be over the threshold, got %s", est)
	}
	if est := estimateSubtitleExtraction(filepath.Join(t.TempDir(), "missing.mkv"), 0, false); est != 0 {
		t.Errorf("a missing file estimates 0, got %s", est)
	}
}

func TestSubtitleRetryAfter_Bounded(t *testing.T) {
	if got := subtitleRetryAfter(10*time.Minute, nil); got != subtitleRetryAfterMax {
		t.Errorf("a long extraction is capped at %s, got %s", subtitleRetryAfterMax, got)
	}
	nearlyDone := &subtitleFlight{started: time.Now().Add(-24 * time.Second), size: 1000}
	if got := subtitleRetryAfter(25*time.Second, nearlyDone); got != subtitleRetryAfterMin {
		t.Errorf("an extraction about to finish floors at %s, got %s", subtitleRetryAfterMin, got)
	}
	if got := subtitleRetryAfter(25*time.Second, nil); got != 25*time.Second {
		t.Errorf("a fresh flight is told the estimate, got %s", got)
	}
}

// Once the walk has moved past the floor, the delay follows the pace actually
// observed, not the estimate: a flight 10 s old that has read a quarter of the
// file has 30 s left, whatever the estimate said.
func TestSubtitleRetryAfter_FollowsRealProgress(t *testing.T) {
	fl := &subtitleFlight{started: time.Now().Add(-10 * time.Second), size: 1000}
	h := fl.open()
	fl.advance(h, 250)
	if got := subtitleRetryAfter(10*time.Minute, fl); got < 28*time.Second || got > subtitleRetryAfterMax {
		t.Errorf("a quarter done in 10 s leaves ~30 s, got %s", got)
	}
	fl.advance(h, 900)
	if got := subtitleRetryAfter(10*time.Minute, fl); got != subtitleRetryAfterMin {
		t.Errorf("90%% done in 10 s leaves ~1 s, floored at %s, got %s", subtitleRetryAfterMin, got)
	}
	// Monotonic: a seek back does not undo progress.
	fl.advance(h, 100)
	if fl.progress() != 0.9 {
		t.Errorf("progress must be monotonic, got %v", fl.progress())
	}
	// Under the floor the estimate still rules.
	young := &subtitleFlight{started: time.Now().Add(-5 * time.Second), size: 1000}
	young.advance(young.open(), 10)
	if got := subtitleRetryAfter(25*time.Second, young); got != 20*time.Second {
		t.Errorf("1%% done: estimate minus elapsed = 20 s, got %s", got)
	}
}

// The reader the extraction opens reports where it is to the flight carried
// by the work context; a seek to the end to size the file is not progress.
func TestPacedReader_ReportsProgressToTheFlight(t *testing.T) {
	path := sparseMKV(t, 1000)
	fl := &subtitleFlight{started: time.Now(), size: 1000}
	ctx := context.WithValue(context.Background(), subtitleFlightCtxKey{}, fl)
	opts := subtitleReadOptions(ctx)
	if len(opts) != 1 || opts[0].FS == nil {
		t.Fatal("a flight in the context must yield a reporting FS, pacer or not")
	}
	f, err := opts[0].FS.DoOpen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	buf := make([]byte, 100)
	if _, rerr := f.Read(buf); rerr != nil {
		t.Fatal(rerr)
	}
	if fl.progress() != 0.1 {
		t.Errorf("after reading 100 of 1000 bytes progress = %v, want 0.1", fl.progress())
	}
	if _, serr := f.Seek(0, io.SeekEnd); serr != nil {
		t.Fatal(serr)
	}
	if fl.progress() != 0.1 {
		t.Errorf("a SeekEnd probe is not progress, got %v", fl.progress())
	}
	if _, serr := f.Seek(500, io.SeekStart); serr != nil {
		t.Fatal(serr)
	}
	if fl.progress() != 0.5 {
		t.Errorf("a seek forward is progress, got %v", fl.progress())
	}
	// A second handle (the walk, opened after the metadata read) supersedes
	// the first: the metadata reader's trip to the Cues at the end of the file
	// must not count as the walk's progress.
	g, gerr := opts[0].FS.DoOpen(path)
	if gerr != nil {
		t.Fatal(gerr)
	}
	defer g.Close()
	if _, serr := f.Seek(990, io.SeekStart); serr != nil {
		t.Fatal(serr)
	}
	if fl.progress() != 0 {
		t.Errorf("a new handle starts the count over and a superseded one must not report, got %v", fl.progress())
	}
	if _, rerr := g.Read(buf[:50]); rerr != nil {
		t.Fatal(rerr)
	}
	if fl.progress() != 0.05 {
		t.Errorf("the new handle read 50 of 1000: progress 0.05, got %v", fl.progress())
	}
	if _, serr := g.Seek(700, io.SeekStart); serr != nil {
		t.Fatal(serr)
	}
	if fl.progress() != 0.7 {
		t.Errorf("the new handle's seek is progress, got %v", fl.progress())
	}
	// Without a flight and without a pacer there is nothing to wrap.
	if opts := subtitleReadOptions(context.Background()); subtitleReadPacer == nil && opts != nil {
		t.Error("no flight, no pacer: mkvgo opens the file itself")
	}
}

// The text route on a film too large to extract within the threshold: 202
// with Retry-After, the extraction started detached. While it runs a second
// request is told the same. When it fails (the file is not a container), the
// failure is memorised and the next request is a 422 at once - under this
// mkvgo version.
func TestSubtitleHandler_LargeFileWithoutIndex_202ThenMemorisedFailure(t *testing.T) {
	cache := newSubtitleCache(t.TempDir())
	filePath := sparseMKV(t, 8<<30)
	item := &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: filePath}}
	h := subtitleHandler(
		&fakeMediaStreamsGetter{item: item, streams: []*domainlibrary.MediaStream{
			{StreamType: domainlibrary.StreamTypeSubtitle, Codec: "subrip", Language: "fre"},
		}},
		"http://localhost", allowAll(), allowAllParental{}, cache,
	)
	do := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitle/0", nil)
		req.SetPathValue("id", "m1")
		req.SetPathValue("track", "0")
		req = withHTTPClaims(req, testClaims(true))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}

	first := do()
	if first.Code != http.StatusAccepted {
		t.Fatalf("a walk of 8 GB must be answered 202, got %d: %s", first.Code, first.Body.String())
	}
	ra, err := strconv.Atoi(first.Header().Get("Retry-After"))
	if err != nil || ra < int(subtitleRetryAfterMin/time.Second) || ra > int(subtitleRetryAfterMax/time.Second) {
		t.Errorf("Retry-After must be a bounded number of seconds, got %q", first.Header().Get("Retry-After"))
	}
	var body struct {
		State         string `json:"state"`
		RetryAfterSec int    `json:"retryAfterSec"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &body); err != nil || body.State != "preparing" || body.RetryAfterSec != ra {
		t.Errorf("body must repeat the state and the delay, got %s", first.Body.String())
	}
	if cc := first.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("a 202 must not be cached, got Cache-Control %q", cc)
	}

	// The detached extraction fails fast (sparse zeros are no container) and
	// leaves a memo; from then on the route answers 422 without a walk.
	cacheKey := cache.keyFor(filePath, 0)
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, failed := cache.getFailure(cacheKey); failed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the detached extraction never recorded its failure")
		}
		time.Sleep(20 * time.Millisecond)
	}
	// The flight may still be unwinding for an instant after the memo lands.
	deadline = time.Now().Add(5 * time.Second)
	var last *httptest.ResponseRecorder
	for {
		last = do()
		if last.Code == http.StatusUnprocessableEntity || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if last.Code != http.StatusUnprocessableEntity {
		t.Fatalf("a memorised failure must be a 422 at once, got %d: %s", last.Code, last.Body.String())
	}
	var fail struct {
		Message string `json:"message"`
		Reason  string `json:"reason"`
	}
	if err := json.Unmarshal(last.Body.Bytes(), &fail); err != nil || fail.Reason == "" {
		t.Errorf("the 422 must carry the memorised reason, got %s", last.Body.String())
	}
}

// A small file is still served inline: no 202 for an extraction that takes a
// second. (The sparse file fails to parse, so the inline path ends in the
// pre-existing 500 - the point is the status is not 202.)
func TestSubtitleHandler_SmallFile_StaysInline(t *testing.T) {
	cache := newSubtitleCache(t.TempDir())
	filePath := sparseMKV(t, 1<<20)
	item := &domainlibrary.MediaItem{ID: "m2", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: filePath}}
	h := subtitleHandler(
		&fakeMediaStreamsGetter{item: item, streams: []*domainlibrary.MediaStream{
			{StreamType: domainlibrary.StreamTypeSubtitle, Codec: "subrip", Language: "fre"},
		}},
		"http://localhost", allowAll(), allowAllParental{}, cache,
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m2/subtitle/0", nil)
	req.SetPathValue("id", "m2")
	req.SetPathValue("track", "0")
	req = withHTTPClaims(req, testClaims(true))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code == http.StatusAccepted {
		t.Fatalf("a 1 MB file must not be answered 202")
	}
}

// The bitmap route answers a memorised failure before touching the file.
func TestServeBitmapSubtitle_MemorisedFailureIs422WithoutARead(t *testing.T) {
	cache := newSubtitleCache(t.TempDir())
	filePath := sparseMKV(t, 8<<30)
	key := cache.keyForKind(filePath, 0, subtitleKindPGS)
	if err := cache.putFailure(key, os.ErrInvalid); err != nil {
		t.Fatal(err)
	}
	item := &domainlibrary.MediaItem{ID: "m3", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: filePath}}
	h := subtitleHandler(
		&fakeMediaStreamsGetter{item: item, streams: []*domainlibrary.MediaStream{
			{StreamType: domainlibrary.StreamTypeSubtitle, Codec: "hdmv_pgs_subtitle"},
		}},
		"http://localhost", allowAll(), allowAllParental{}, cache,
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m3/subtitle/0", nil)
	req.SetPathValue("id", "m3")
	req.SetPathValue("track", "0")
	req = withHTTPClaims(req, testClaims(true))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("memorised PGS failure must 422, got %d: %s", w.Code, w.Body.String())
	}
}
