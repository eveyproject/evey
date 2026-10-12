package httpapi

import (
	"context"
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

// countingArtworkWriter counts UpdateWebPPath calls with an atomic counter
// rather than the plain fields fakeArtworkWriter uses: this test drives
// concurrent requests at the SAME rec on purpose, and an unsynchronized
// counter would itself race whenever the fix under test fails to coalesce.
type countingArtworkWriter struct {
	calls atomic.Int32
}

func (c *countingArtworkWriter) UpdateWebPPath(_ context.Context, _, _ string) error {
	c.calls.Add(1)
	return nil
}

// TestArtwork_WebPSingleFlight_ConcurrentSameImage proves that N concurrent
// requests for the SAME image's full-resolution WebP (no ?w=, so the
// resolveWebP path, not the resize path) trigger only ONE ConvertToWebP call -
// the others must wait on it instead of each racing to convert and write the
// identical destination file.
func TestArtwork_WebPSingleFlight_ConcurrentSameImage(t *testing.T) {
	tmp := t.TempDir()
	imgPath := filepath.Join(tmp, "poster.jpg")
	if err := os.WriteFile(imgPath, []byte("orig"), 0644); err != nil {
		t.Fatal(err)
	}
	localPath := imgPath
	conv := newBlockingImageConverter()
	writer := &countingArtworkWriter{}

	h := mediaHandler(
		&fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1"}},
		&fakeArtworkRepo{records: map[string][]*domainmeta.ArtworkRecord{
			"m1": {{ID: "a1", MediaItemID: "m1", Type: "poster", LocalPath: &localPath}},
		}},
		"http://localhost",
		staticLibraryAccessChecker{},
		withImageConverter(conv, writer),
	)

	const N = 10
	var wg sync.WaitGroup
	codes := make([]int, N)
	for i := range N {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// No ?w= - this must hit resolveWebP's full-resolution conversion,
			// never the resize path.
			req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/artwork/poster", nil)
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
	case <-conv.webpEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the leader to start converting")
	}
	// No SECOND concurrent entry: every other request must be parked, not each
	// launching its own conversion of the same source image.
	select {
	case <-conv.webpEntered:
		t.Fatal("a second concurrent request triggered its own ConvertToWebP - duplicates are not collapsed")
	case <-time.After(200 * time.Millisecond):
	}
	close(conv.webpRelease)
	wg.Wait()

	if calls := conv.webpCalls.Load(); calls != 1 {
		t.Errorf("expected exactly 1 ConvertToWebP call for %d concurrent identical requests, got %d", N, calls)
	}
	if calls := writer.calls.Load(); calls != 1 {
		t.Errorf("expected exactly 1 UpdateWebPPath persist call, got %d", calls)
	}
	for i, code := range codes {
		if code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i, code)
		}
	}
}

// TestArtwork_WebPConcurrency_BoundedBySemaphore proves that concurrent
// full-resolution WebP conversions for DIFFERENT images (so single-flight
// can't collapse them) are still capped at artworkResizeMaxConcurrent
// in-flight ffmpeg calls at once - sharing the SAME bound the resize path
// uses, since it is the same limited ffmpeg budget on the same weak NAS.
func TestArtwork_WebPConcurrency_BoundedBySemaphore(t *testing.T) {
	const N = 5
	if N <= artworkResizeMaxConcurrent {
		t.Fatalf("test requires N > artworkResizeMaxConcurrent to prove the bound, got N=%d cap=%d", N, artworkResizeMaxConcurrent)
	}
	tmp := t.TempDir()
	conv := newBlockingImageConverter()
	handlers := make([]http.Handler, N)
	for i := range N {
		imgPath := filepath.Join(tmp, "poster"+string(rune('a'+i))+".jpg")
		if err := os.WriteFile(imgPath, []byte("orig"), 0644); err != nil {
			t.Fatal(err)
		}
		localPath := imgPath
		id := "m" + string(rune('a'+i))
		handlers[i] = mediaHandler(
			&fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: shared.MediaItemID(id), LibraryID: "lib1"}},
			&fakeArtworkRepo{records: map[string][]*domainmeta.ArtworkRecord{
				id: {{ID: "a" + id, MediaItemID: shared.MediaItemID(id), Type: "poster", LocalPath: &localPath}},
			}},
			"http://localhost",
			staticLibraryAccessChecker{},
			withImageConverter(conv, &countingArtworkWriter{}),
		)
	}

	var wg sync.WaitGroup
	for i := range N {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := "m" + string(rune('a'+i))
			req := httptest.NewRequest(http.MethodGet, "/api/v1/media/"+id+"/artwork/poster", nil)
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

	for i := range artworkResizeMaxConcurrent {
		select {
		case <-conv.webpEntered:
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for concurrent slot %d/%d - bound not reached", i+1, artworkResizeMaxConcurrent)
		}
	}
	select {
	case <-conv.webpEntered:
		t.Fatalf("more than %d WebP conversions ran concurrently - semaphore bound not enforced", artworkResizeMaxConcurrent)
	case <-time.After(200 * time.Millisecond):
	}
	close(conv.webpRelease)
	wg.Wait()

	if calls := conv.webpCalls.Load(); calls != int32(N) {
		t.Errorf("expected %d ConvertToWebP calls (distinct images), got %d", N, calls)
	}
	if peak := conv.webpPeak.Load(); peak > int32(artworkResizeMaxConcurrent) {
		t.Errorf("peak concurrency %d exceeded bound %d", peak, artworkResizeMaxConcurrent)
	}
}
