package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	domainmeta "github.com/eveyproject/evey/internal/domain/metadata"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// staticLibraryAccessChecker is a stateless allow-all LibraryAccessChecker.
// The shared allowAll() fake (helpers_test.go) records "called" in a plain
// bool with no synchronization - fine for the sequential tests it was built
// for, but a data race once N goroutines hit the SAME instance concurrently,
// as these tests deliberately do. Sidestepping it here avoids touching a
// widely-used shared test fake just to add a mutex it doesn't otherwise need.
type staticLibraryAccessChecker struct{}

func (staticLibraryAccessChecker) HasAccess(_ context.Context, _ shared.UserID, _ shared.LibraryID) (bool, error) {
	return true, nil
}

// blockingImageConverter simulates a real ffmpeg resize: ResizeAndConvertToWebP
// blocks until release is closed, signalling entry on entered first. This lets
// a test observe exactly how many resize calls are in flight at once, and how
// many total calls were made - the two properties single-flight and the
// concurrency bound must respectively collapse and cap.
//
// ConvertToWebP (the full-resolution, non-resized path) gets its OWN
// entered/release/counters below: a test exercising the resize path only
// (the two tests in this file) must never have its "exactly one entry"
// assertions perturbed by an unrelated ConvertToWebP call, and vice versa.
type blockingImageConverter struct {
	entered chan struct{}
	release chan struct{}
	calls   atomic.Int32
	peak    atomic.Int32
	current atomic.Int32

	webpEntered chan struct{}
	webpRelease chan struct{}
	webpCalls   atomic.Int32
	webpPeak    atomic.Int32
	webpCurrent atomic.Int32
}

func newBlockingImageConverter() *blockingImageConverter {
	return &blockingImageConverter{
		entered:     make(chan struct{}, 64),
		release:     make(chan struct{}),
		webpEntered: make(chan struct{}, 64),
		webpRelease: make(chan struct{}),
	}
}

func (c *blockingImageConverter) ConvertToWebP(_ context.Context, _, dst string, _ int) error {
	c.webpCalls.Add(1)
	cur := c.webpCurrent.Add(1)
	for {
		p := c.webpPeak.Load()
		if cur <= p {
			break
		}
		if c.webpPeak.CompareAndSwap(p, cur) {
			break
		}
	}
	c.webpEntered <- struct{}{}
	<-c.webpRelease
	c.webpCurrent.Add(-1)
	return os.WriteFile(dst, []byte("full"), 0644)
}

func (c *blockingImageConverter) ResizeAndConvertToWebP(_ context.Context, _, dst string, _, _ int) error {
	c.calls.Add(1)
	cur := c.current.Add(1)
	for {
		p := c.peak.Load()
		if cur <= p {
			break
		}
		if c.peak.CompareAndSwap(p, cur) {
			break
		}
	}
	c.entered <- struct{}{}
	<-c.release
	c.current.Add(-1)
	return os.WriteFile(dst, []byte("resized"), 0644)
}

// TestArtwork_ResizeSingleFlight_ConcurrentSameVariant proves that N concurrent
// requests for the SAME resized variant (same media, same width) trigger only
// ONE ffmpeg invocation - the others wait on it instead of each racing to
// generate and write the identical dstPath.
func TestArtwork_ResizeSingleFlight_ConcurrentSameVariant(t *testing.T) {
	tmp := t.TempDir()
	imgPath := filepath.Join(tmp, "poster.jpg")
	if err := os.WriteFile(imgPath, []byte("orig"), 0644); err != nil {
		t.Fatal(err)
	}
	localPath := imgPath
	conv := newBlockingImageConverter()

	h := mediaHandler(
		&fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1"}},
		&fakeArtworkRepo{records: map[string][]*domainmeta.ArtworkRecord{
			"m1": {{ID: "a1", MediaItemID: "m1", Type: "poster", LocalPath: &localPath}},
		}},
		"http://localhost",
		staticLibraryAccessChecker{},
		withImageConverter(conv, &fakeArtworkWriter{}),
	)

	const N = 10
	var wg sync.WaitGroup
	codes := make([]int, N)
	for i := range N {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/artwork/poster?w=342", nil)
			req.SetPathValue("id", "m1")
			req.SetPathValue("artType", "poster")
			req = withHTTPClaims(req, testClaims(false))
			req.Header.Set("Accept", "image/webp")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			codes[i] = w.Code
		}(i)
	}

	// Exactly one request should reach the converter (the single-flight leader).
	select {
	case <-conv.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the leader to start resizing")
	}
	// No SECOND concurrent entry: every other request must be parked on the
	// single-flight group, not each launching its own resize.
	select {
	case <-conv.entered:
		t.Fatal("a second concurrent request triggered its own resize - duplicates are not collapsed")
	case <-time.After(200 * time.Millisecond):
	}
	close(conv.release)
	wg.Wait()

	if calls := conv.calls.Load(); calls != 1 {
		t.Errorf("expected exactly 1 resize call for %d concurrent identical requests, got %d", N, calls)
	}
	for i, code := range codes {
		if code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i, code)
		}
	}
}

// TestArtwork_ResizeConcurrency_BoundedBySemaphore proves that concurrent
// resizes for DIFFERENT images (so single-flight can't collapse them) are
// still capped at artworkResizeMaxConcurrent in-flight ffmpeg calls at once -
// a cold poster grid must not spawn one ffmpeg per thumbnail on the weak NAS
// this targets.
func TestArtwork_ResizeConcurrency_BoundedBySemaphore(t *testing.T) {
	const N = 5
	if N <= artworkResizeMaxConcurrent {
		t.Fatalf("test requires N > artworkResizeMaxConcurrent to prove the bound, got N=%d cap=%d", N, artworkResizeMaxConcurrent)
	}
	tmp := t.TempDir()
	conv := newBlockingImageConverter()
	handlers := make([]http.Handler, N)
	for i := range N {
		imgPath := filepath.Join(tmp, fmt.Sprintf("poster%d.jpg", i))
		if err := os.WriteFile(imgPath, []byte("orig"), 0644); err != nil {
			t.Fatal(err)
		}
		localPath := imgPath
		id := fmt.Sprintf("m%d", i)
		handlers[i] = mediaHandler(
			&fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: shared.MediaItemID(id), LibraryID: "lib1"}},
			&fakeArtworkRepo{records: map[string][]*domainmeta.ArtworkRecord{
				id: {{ID: "a" + id, MediaItemID: shared.MediaItemID(id), Type: "poster", LocalPath: &localPath}},
			}},
			"http://localhost",
			staticLibraryAccessChecker{},
			withImageConverter(conv, &fakeArtworkWriter{}),
		)
	}

	var wg sync.WaitGroup
	for i := range N {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("m%d", i)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/media/"+id+"/artwork/poster?w=342", nil)
			req.SetPathValue("id", id)
			req.SetPathValue("artType", "poster")
			req = withHTTPClaims(req, testClaims(false))
			req.Header.Set("Accept", "image/webp")
			w := httptest.NewRecorder()
			handlers[i].ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Errorf("request %d: expected 200, got %d: %s", i, w.Code, w.Body.String())
			}
		}(i)
	}

	// Exactly artworkResizeMaxConcurrent requests should be able to enter the
	// converter concurrently.
	for i := range artworkResizeMaxConcurrent {
		select {
		case <-conv.entered:
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for concurrent slot %d/%d - bound not reached", i+1, artworkResizeMaxConcurrent)
		}
	}
	// One more must NOT get through: the bound must not be exceeded.
	select {
	case <-conv.entered:
		t.Fatalf("more than %d resizes ran concurrently - semaphore bound not enforced", artworkResizeMaxConcurrent)
	case <-time.After(200 * time.Millisecond):
	}
	close(conv.release)
	wg.Wait()

	if calls := conv.calls.Load(); calls != int32(N) {
		t.Errorf("expected %d resize calls (distinct images), got %d", N, calls)
	}
	if peak := conv.peak.Load(); peak > int32(artworkResizeMaxConcurrent) {
		t.Errorf("peak concurrency %d exceeded bound %d", peak, artworkResizeMaxConcurrent)
	}
}
