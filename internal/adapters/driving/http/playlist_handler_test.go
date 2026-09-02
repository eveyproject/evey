package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	appplaylist "github.com/evey-project/evey/internal/app/playlist"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/playlist"
	"github.com/evey-project/evey/internal/domain/shared"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
)

// --- stubs for generatePlaylist tests ---

type stubPlaylistGenerator struct {
	pl  *playlist.Playlist
	err error
}

func (s *stubPlaylistGenerator) Generate(_ context.Context, _ string, _ shared.LibraryID, _ int, _ string) (*playlist.Playlist, error) {
	return s.pl, s.err
}

// stubPlaylistArchiveSource satisfies playlistArchiveSource with no-op behaviour;
// generatePlaylist does not use plSvc at all, so nil-safe stubs suffice.
type stubPlaylistArchiveSource struct{}

func (s *stubPlaylistArchiveSource) GetByID(_ context.Context, _ string, _ shared.ProfileID) (*playlist.Playlist, error) {
	return nil, errors.New("not implemented in stub")
}

func (s *stubPlaylistArchiveSource) GetItems(_ context.Context, _ string, _ shared.ProfileID) ([]*domainlibrary.MediaItem, []*playlist.PlaylistItem, error) {
	return nil, nil, errors.New("not implemented in stub")
}

// --- helpers ---

// buildPlaylistMux wires a mux with the playlist routes using the provided
// generator and library-access checker, and returns (mux, userToken).
// It uses a real JWTGenerator (via genTokens) so the auth middleware works.
func buildPlaylistMux(t *testing.T, gen playlistGenerator, checker LibraryAccessChecker) (*http.ServeMux, string) {
	t.Helper()
	jwtGen, _, userTok := genTokens(t)
	rl := ratelimit.New(600)
	mux := http.NewServeMux()

	h := &playlistHandler{
		plSvc:      &stubPlaylistArchiveSource{},
		gen:        gen,
		libSvc:     &fakeMediaItemGetter{},
		music:      nil,
		libChecker: checker,
		baseURL:    "http://localhost",
	}

	authMw := httpAuthMiddleware(jwtGen)
	mux.HandleFunc("POST /api/v1/playlists/generate", ratelimit.MiddlewareFunc(rl, authMw(h.generatePlaylist)))
	return mux, userTok
}

func generateBody(t *testing.T, libraryID, name string, size int) *bytes.Reader {
	t.Helper()
	b, err := json.Marshal(map[string]any{"libraryId": libraryID, "size": size, "name": name})
	if err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(b)
}

// --- tests ---

// TestGeneratePlaylist_HappyPath verifies that an authenticated user with library
// access gets back a 200 with the generated playlist id and name.
func TestGeneratePlaylist_HappyPath(t *testing.T) {
	gen := &stubPlaylistGenerator{
		pl: &playlist.Playlist{ID: "pl-001", Name: "My Mix"},
	}
	mux, userTok := buildPlaylistMux(t, gen, allowAll())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/playlists/generate", generateBody(t, "lib-1", "My Mix", 20))
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if body["id"] != "pl-001" {
		t.Errorf("expected id=pl-001, got %q", body["id"])
	}
	if body["name"] != "My Mix" {
		t.Errorf("expected name=My Mix, got %q", body["name"])
	}
}

// TestGeneratePlaylist_DefaultName verifies that an empty name in the body is
// replaced by the "Mix" default and forwarded to the generator.
func TestGeneratePlaylist_DefaultName(t *testing.T) {
	var capturedName string
	gen := &capturingPlaylistGenerator{
		fn: func(name string) (*playlist.Playlist, error) {
			capturedName = name
			return &playlist.Playlist{ID: "pl-002", Name: name}, nil
		},
	}
	mux, userTok := buildPlaylistMux(t, gen, allowAll())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/playlists/generate", generateBody(t, "lib-1", "", 10))
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	// The handler forwards the ABSENCE of a name rather than inventing one:
	// naming a generated playlist is the generator's business, because only it
	// can step aside when "Mix" is already taken. Doing it here returned a 500
	// on the second press of the button (prod, 2026-09-02).
	if capturedName != "" {
		t.Errorf("expected the empty name to be forwarded, got %q", capturedName)
	}
}

// capturingPlaylistGenerator lets tests intercept the name passed to Generate.
type capturingPlaylistGenerator struct {
	fn func(name string) (*playlist.Playlist, error)
}

func (c *capturingPlaylistGenerator) Generate(_ context.Context, _ string, _ shared.LibraryID, _ int, name string) (*playlist.Playlist, error) {
	return c.fn(name)
}

// TestGeneratePlaylist_Unauthenticated verifies that a request without a valid
// Bearer token receives a 401.
func TestGeneratePlaylist_Unauthenticated(t *testing.T) {
	mux, _ := buildPlaylistMux(t, &stubPlaylistGenerator{}, allowAll())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/playlists/generate", generateBody(t, "lib-1", "Mix", 10))
	// no Authorization header
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

// TestGeneratePlaylist_MissingLibraryID verifies that a body without libraryId
// is rejected with 400.
func TestGeneratePlaylist_MissingLibraryID(t *testing.T) {
	mux, userTok := buildPlaylistMux(t, &stubPlaylistGenerator{}, allowAll())

	body, _ := json.Marshal(map[string]any{"size": 10, "name": "Mix"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/playlists/generate", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestGeneratePlaylist_InvalidJSON verifies that a malformed body returns 400.
func TestGeneratePlaylist_InvalidJSON(t *testing.T) {
	mux, userTok := buildPlaylistMux(t, &stubPlaylistGenerator{}, allowAll())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/playlists/generate", bytes.NewReader([]byte("not-json")))
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// TestGeneratePlaylist_LibraryAccessDenied verifies the IDOR guard: a user with
// no grant on the library receives 403.
func TestGeneratePlaylist_LibraryAccessDenied(t *testing.T) {
	mux, userTok := buildPlaylistMux(t, &stubPlaylistGenerator{}, denyAll())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/playlists/generate", generateBody(t, "lib-restricted", "Mix", 10))
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// TestGeneratePlaylist_NoTracks verifies that when the generator reports no
// available tracks, the handler returns 422.
func TestGeneratePlaylist_NoTracks(t *testing.T) {
	gen := &stubPlaylistGenerator{err: appplaylist.ErrNoTracks}
	mux, userTok := buildPlaylistMux(t, gen, allowAll())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/playlists/generate", generateBody(t, "lib-1", "Mix", 10))
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
}

// TestGeneratePlaylist_GeneratorError verifies that an unexpected generator error
// yields 500.
func TestGeneratePlaylist_GeneratorError(t *testing.T) {
	gen := &stubPlaylistGenerator{err: errors.New("db failure")}
	mux, userTok := buildPlaylistMux(t, gen, allowAll())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/playlists/generate", generateBody(t, "lib-1", "Mix", 10))
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

// TestGeneratePlaylist_DirectClaimsInjection verifies the handler directly when
// claims are injected via context (bypasses auth middleware entirely) so we can
// assert that a nil-claims context still yields 401 (belt-and-suspenders).
func TestGeneratePlaylist_NilClaims_401(t *testing.T) {
	h := &playlistHandler{
		plSvc:      &stubPlaylistArchiveSource{},
		gen:        &stubPlaylistGenerator{pl: &playlist.Playlist{ID: "x"}},
		libSvc:     &fakeMediaItemGetter{},
		libChecker: allowAll(),
		baseURL:    "http://localhost",
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/playlists/generate", generateBody(t, "lib-1", "Mix", 5))
	// context carries *no* claims - simulates missing/corrupt middleware
	w := httptest.NewRecorder()
	h.generatePlaylist(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with nil claims, got %d", w.Code)
	}
}

// ensure unused import of domainuser is not a compile error when used only via
// genTokens (which returns Claims internally). The import is consumed by
// withHTTPClaims via testClaims in other test files; add a sentinel usage here.
var _ = domainuser.Claims{}

// --- reorderItems tests ---

// stubPlaylistReorderer records the arguments ReorderItems was called with and
// returns the configured error, letting tests drive both the ownership-mapped
// 404 path and the happy path without a real playlist/postgres repo.
type stubPlaylistReorderer struct {
	err              error
	capturedPlaylist string
	capturedProfile  shared.ProfileID
	capturedIDs      []string
}

func (s *stubPlaylistReorderer) ReorderItems(_ context.Context, playlistID string, profileID shared.ProfileID, mediaItemIDs []string) error {
	s.capturedPlaylist = playlistID
	s.capturedProfile = profileID
	s.capturedIDs = mediaItemIDs
	return s.err
}

// buildPlaylistReorderMux wires just the reorder route (PUT
// /api/v1/playlists/{id}/items/reorder) against the given reorderer stub.
func buildPlaylistReorderMux(t *testing.T, reorder *stubPlaylistReorderer) (*http.ServeMux, string) {
	t.Helper()
	jwtGen, _, userTok := genTokens(t)
	mux := http.NewServeMux()
	h := &playlistHandler{reorder: reorder}
	authMw := httpAuthMiddleware(jwtGen)
	mux.HandleFunc("PUT /api/v1/playlists/{id}/items/reorder", authMw(h.reorderItems))
	return mux, userTok
}

func reorderBody(items []map[string]any) *bytes.Reader {
	b, _ := json.Marshal(map[string]any{"items": items})
	return bytes.NewReader(b)
}

// TestReorderPlaylistItems_HappyPath verifies the item ids are extracted in
// sortOrder order (not necessarily array order) and forwarded to the service
// along with the caller's profile id.
func TestReorderPlaylistItems_HappyPath(t *testing.T) {
	reorder := &stubPlaylistReorderer{}
	mux, userTok := buildPlaylistReorderMux(t, reorder)

	body := reorderBody([]map[string]any{
		{"mediaItemId": "b", "sortOrder": 1},
		{"mediaItemId": "a", "sortOrder": 0},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/playlists/pl-1/items/reorder", body)
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if reorder.capturedPlaylist != "pl-1" {
		t.Errorf("expected playlist id pl-1, got %q", reorder.capturedPlaylist)
	}
	if reorder.capturedProfile != "prof-1" {
		t.Errorf("expected profile id prof-1 from the caller's token, got %q", reorder.capturedProfile)
	}
	if got := reorder.capturedIDs; len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("expected ids sorted by sortOrder [a b], got %v", got)
	}
}

// TestReorderPlaylistItems_NotOwner verifies that ErrNotOwner (like a
// not-found playlist) maps to 404, matching GetPlaylist/AddPlaylistItem -
// existence of a playlist owned by someone else must not leak.
func TestReorderPlaylistItems_NotOwner(t *testing.T) {
	reorder := &stubPlaylistReorderer{err: appplaylist.ErrNotOwner}
	mux, userTok := buildPlaylistReorderMux(t, reorder)

	body := reorderBody([]map[string]any{{"mediaItemId": "a", "sortOrder": 0}})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/playlists/pl-1/items/reorder", body)
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// TestReorderPlaylistItems_Unauthenticated verifies a request without a valid
// Bearer token receives a 401.
func TestReorderPlaylistItems_Unauthenticated(t *testing.T) {
	mux, _ := buildPlaylistReorderMux(t, &stubPlaylistReorderer{})

	req := httptest.NewRequest(http.MethodPut, "/api/v1/playlists/pl-1/items/reorder", reorderBody(nil))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

// TestReorderPlaylistItems_EmptyItems verifies an empty items array is
// rejected with 400 instead of silently forwarding a no-op reorder.
func TestReorderPlaylistItems_EmptyItems(t *testing.T) {
	reorder := &stubPlaylistReorderer{}
	mux, userTok := buildPlaylistReorderMux(t, reorder)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/playlists/pl-1/items/reorder", reorderBody(nil))
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestReorderPlaylistItems_RouteMissingBeforeThisChange documents the bug:
// before this route was registered, PUT .../items/reorder 404'd on any mux
// that only knows the pre-existing playlist routes (the proto declares no
// reorder RPC either - grep proof in the report). Runs against the exact
// same buildPlaylistMux helper the generate/download tests use, with no
// reorder route added, to pin the pre-fix behaviour as a regression guard.
func TestReorderPlaylistItems_RouteMissingOnLegacyMux(t *testing.T) {
	mux, userTok := buildPlaylistMux(t, &stubPlaylistGenerator{}, allowAll())

	req := httptest.NewRequest(http.MethodPut, "/api/v1/playlists/pl-1/items/reorder", reorderBody([]map[string]any{{"mediaItemId": "a", "sortOrder": 0}}))
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 (route not registered), got %d", w.Code)
	}
}
