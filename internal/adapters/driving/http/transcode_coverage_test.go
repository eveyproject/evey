package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// --- shared helpers ---

// ============================================================
// HLS cache tests
// ============================================================

func TestCachedLibraryAccessChecker_CacheHit(t *testing.T) {
	inner := allowAll()
	cached := NewCachedLibraryAccessChecker(inner)
	defer cached.Close()

	ok, err := cached.HasAccess(context.Background(), "u1", "lib1")
	if err != nil || !ok {
		t.Fatalf("expected access, got ok=%v err=%v", ok, err)
	}
	// second call with inner changed to deny - should still return cached true
	inner.allowed = false
	ok2, _ := cached.HasAccess(context.Background(), "u1", "lib1")
	if !ok2 {
		t.Error("expected cached access=true")
	}
}

func TestCachedLibraryAccessChecker_DeniedCacheHit(t *testing.T) {
	inner := denyAll()
	cached := NewCachedLibraryAccessChecker(inner)
	defer cached.Close()

	ok, err := cached.HasAccess(context.Background(), "u1", "lib1")
	if err != nil || ok {
		t.Fatalf("expected denied, got ok=%v err=%v", ok, err)
	}
}

func TestCachedLibraryAccessChecker_Error(t *testing.T) {
	inner := &fakeLibraryAccessChecker{err: errors.New("db error")}
	cached := NewCachedLibraryAccessChecker(inner)
	defer cached.Close()

	_, err := cached.HasAccess(context.Background(), "u1", "lib1")
	if err == nil {
		t.Error("expected error")
	}
}

// claimsFromContext tests removed (share_handler migrated to gRPC)

// ============================================================
// transcode parseParams tests
// ============================================================

// ============================================================
// transcode analyzeStreams tests
// ============================================================

func TestHDRQuerySupported_AcceptsBothClientForms(t *testing.T) {
	cases := map[string]bool{"/t?hdr=1": true, "/t?hdr=true": true, "/t?hdr=0": false, "/t?hdr=false": false, "/t": false}
	for target, want := range cases {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		if got := hdrQuerySupported(req); got != want {
			t.Errorf("hdrQuerySupported(%q) = %v, want %v", target, got, want)
		}
	}
}

// ============================================================
// Subtitle list with items
// ============================================================

// ============================================================
// HLS segment handler auth tests
// ============================================================

// HLSSegmentHandler_QueryToken skipped - requires non-nil HLSManager to avoid panic after auth

// ============================================================
// Transcode handler auth tests
// ============================================================
