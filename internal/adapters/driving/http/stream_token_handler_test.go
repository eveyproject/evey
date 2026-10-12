package httpapi

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appmedia "github.com/eveyproject/evey/internal/app/media"
	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/auth"
)

func TestStreamTokenHandler_CreateToken(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)

	mg := newFakeMediaGetter()
	mg.items["m1"] = &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1"}
	store := appmedia.NewStreamTokenStore(5 * time.Minute)

	h := streamTokenHandler(gen, store, allowAll(), mg, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/stream-token", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, &domainuser.Claims{UserID: "u1", IsAdmin: false})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["token"] == "" {
		t.Error("expected non-empty token")
	}
	if resp["mediaId"] != "m1" {
		t.Errorf("expected mediaId=m1, got %s", resp["mediaId"])
	}
}

func TestStreamTokenHandler_InvalidToken_401(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	store := appmedia.NewStreamTokenStore(5 * time.Minute)

	inner := streamTokenHandler(gen, store, allowAll(), newFakeMediaGetter(), nil)
	h := httpAuthMiddleware(gen)(inner)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/stream-token", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestStreamTokenHandler_NoAuth_401(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	store := appmedia.NewStreamTokenStore(5 * time.Minute)

	inner := streamTokenHandler(gen, store, allowAll(), newFakeMediaGetter(), nil)
	h := httpAuthMiddleware(gen)(inner)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/stream-token", nil)
	req.SetPathValue("id", "m1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestStreamTokenHandler_MediaNotFound_404(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	store := appmedia.NewStreamTokenStore(5 * time.Minute)

	h := streamTokenHandler(gen, store, allowAll(), newFakeMediaGetter(), nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/stream-token", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, &domainuser.Claims{UserID: "u1", IsAdmin: false})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// Plan B P2: ?purpose=native mints the same long-lived (12h) token as cast, so a
// native <video> HLS element - which can't re-mint the sliding token mid-stream -
// survives pauses; the default (no purpose) stays on the short 5-minute window.
func TestStreamTokenHandler_NativePurpose_LongTTL(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	mg := newFakeMediaGetter()
	mg.items["m1"] = &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1"}

	mintTTL := func(purpose string) time.Duration {
		store := appmedia.NewStreamTokenStore(appmedia.DefaultStreamTokenTTL)
		h := streamTokenHandler(gen, store, allowAll(), mg, nil)
		url := "/api/v1/media/m1/stream-token"
		if purpose != "" {
			url += "?purpose=" + purpose
		}
		req := httptest.NewRequest(http.MethodPost, url, nil)
		req.SetPathValue("id", "m1")
		req = withHTTPClaims(req, &domainuser.Claims{UserID: "u1"})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("purpose %q: status %d: %s", purpose, w.Code, w.Body.String())
		}
		var resp map[string]string
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("purpose %q: decode: %v", purpose, err)
		}
		st, ok := store.ValidateAny(resp["token"])
		if !ok {
			t.Fatalf("purpose %q: minted token did not validate", purpose)
		}
		return st.TTL
	}

	if got := mintTTL("native"); got != appmedia.CastStreamTokenTTL {
		t.Errorf("native token TTL = %v, want %v (long-lived, matches cast)", got, appmedia.CastStreamTokenTTL)
	}
	if got := mintTTL("cast"); got != appmedia.CastStreamTokenTTL {
		t.Errorf("cast token TTL = %v, want %v", got, appmedia.CastStreamTokenTTL)
	}
	if got := mintTTL(""); got != appmedia.DefaultStreamTokenTTL {
		t.Errorf("default token TTL = %v, want %v (short sliding)", got, appmedia.DefaultStreamTokenTTL)
	}
}

func TestStreamTokenHandler_IDOR_403(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	mg := newFakeMediaGetter()
	mg.items["m1"] = &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib-restricted"}
	store := appmedia.NewStreamTokenStore(5 * time.Minute)

	h := streamTokenHandler(gen, store, denyAll(), mg, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/stream-token", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, &domainuser.Claims{UserID: "u1", IsAdmin: false})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
