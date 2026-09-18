package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

// Edge: URI already carries a query string - must use & not ? a second time.

// Edge: CRLF line endings must survive so strict TV clients still parse.

// ============================================================
// parseClientCapabilities (pure function, was 0% covered)
// ============================================================

func TestParseClientCapabilities_Defaults(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	c := parseClientCapabilities(req, false)
	if c.SupportsHDR {
		t.Error("expected SupportsHDR=false by default")
	}
}

func TestParseClientCapabilities_HDRSupportedFlag(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	c := parseClientCapabilities(req, true)
	if !c.SupportsHDR {
		t.Error("expected SupportsHDR=true when flag passed")
	}
}

func TestParseClientCapabilities_AllParams(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test?videoCodecs=hevc,av1&audioCodecs=opus,aac&containers=mkv,mp4", nil)
	c := parseClientCapabilities(req, false)
	if len(c.VideoCodecs) != 2 || c.VideoCodecs[0] != "hevc" {
		t.Errorf("expected hevc,av1 codec list, got %v", c.VideoCodecs)
	}
	if len(c.AudioCodecs) != 2 || c.AudioCodecs[1] != "aac" {
		t.Errorf("expected opus,aac codec list, got %v", c.AudioCodecs)
	}
	if len(c.Containers) != 2 || c.Containers[0] != "mkv" {
		t.Errorf("expected mkv,mp4 containers, got %v", c.Containers)
	}
}

// ============================================================
// loadProfilePrefs (28.6% - exercise nil/empty/error/success branches)
// ============================================================

type fakeProfilePrefsGetter struct {
	prefs map[string]string
	err   error
}

func (f *fakeProfilePrefsGetter) GetProfilePreferences(_ context.Context, _ string) (map[string]string, error) {
	return f.prefs, f.err
}

func TestLoadProfilePrefs_NilGetter_ReturnsDefaults(t *testing.T) {
	p := loadProfilePrefs(context.Background(), nil, "any")
	def := apptranscoding.DefaultTranscodingPreferences()
	if !reflect.DeepEqual(p, def) {
		t.Errorf("expected defaults when getter is nil, got %+v", p)
	}
}

func TestLoadProfilePrefs_EmptyProfileID_ReturnsDefaults(t *testing.T) {
	getter := &fakeProfilePrefsGetter{prefs: map[string]string{"allow_direct_play": "false"}}
	p := loadProfilePrefs(context.Background(), getter, "")
	def := apptranscoding.DefaultTranscodingPreferences()
	if !reflect.DeepEqual(p, def) {
		t.Errorf("expected defaults when profileID is empty, got %+v", p)
	}
}

func TestLoadProfilePrefs_ErrorFallsBackToDefaults(t *testing.T) {
	getter := &fakeProfilePrefsGetter{err: errors.New("db down")}
	p := loadProfilePrefs(context.Background(), getter, "p1")
	def := apptranscoding.DefaultTranscodingPreferences()
	if !reflect.DeepEqual(p, def) {
		t.Errorf("expected defaults on getter error, got %+v", p)
	}
}

func TestLoadProfilePrefs_SuccessParses(t *testing.T) {
	getter := &fakeProfilePrefsGetter{prefs: map[string]string{
		"preferred_video_codec": "hevc",
		"max_audio_channels":    "2",
		"allow_direct_play":     "false",
	}}
	p := loadProfilePrefs(context.Background(), getter, "p1")
	if p.PreferredVideoCodec != "hevc" {
		t.Errorf("expected hevc, got %q", p.PreferredVideoCodec)
	}
	if p.MaxAudioChannels != 2 {
		t.Errorf("expected MaxAudioChannels=2, got %d", p.MaxAudioChannels)
	}
	if p.AllowDirectPlay {
		t.Error("expected AllowDirectPlay=false")
	}
}

// ============================================================
// libraryItemCounter.CountByLibrary (was 0% covered)
// ============================================================
// We exercise the nil-service branches (returns 0, nil) to guard the
// dispatch logic without requiring concrete service wiring.

func TestLibraryItemCounter_Music_NilService_ReturnsZero(t *testing.T) {
	c := &libraryItemCounter{}
	n, err := c.CountByLibrary(context.Background(), shared.LibraryID("lib1"), "music")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0, got %d", n)
	}
}

func TestLibraryItemCounter_Photo_NilService_ReturnsZero(t *testing.T) {
	c := &libraryItemCounter{}
	n, err := c.CountByLibrary(context.Background(), shared.LibraryID("lib1"), "photo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0, got %d", n)
	}
}

func TestLibraryItemCounter_Default_NilService_ReturnsZero(t *testing.T) {
	c := &libraryItemCounter{}
	n, err := c.CountByLibrary(context.Background(), shared.LibraryID("lib1"), "movie")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0, got %d", n)
	}
}

func TestLibraryItemCounter_TvshowDispatch_NilService_ReturnsZero(t *testing.T) {
	c := &libraryItemCounter{}
	n, err := c.CountByLibrary(context.Background(), shared.LibraryID("lib1"), "tvshow")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0, got %d", n)
	}
}

func TestNewLibraryItemCounter_NotNil(t *testing.T) {
	c := NewLibraryItemCounter(nil, nil, nil)
	if c == nil {
		t.Fatal("expected non-nil counter")
	}
	// dispatch still safe with nil services
	if _, err := c.CountByLibrary(context.Background(), shared.LibraryID("lib1"), "music"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// ============================================================
// Stream-token mismatch -> 403 across handlers using StreamTokenFromContext.
// These cover the early-return branches that were skipped by JWT-based tests.
// ============================================================

func TestChaptersHandler_StreamToken_WrongMediaID_403(t *testing.T) {
	h := chaptersHandler(
		&fakeChaptersGetter{},
		"http://localhost",
		allowAll(),
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		nil,
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/chapters", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	ctx := context.WithValue(req.Context(), streamTokenCtxKey, &shared.StreamToken{
		UserID:      "u1",
		MediaItemID: "other-media",
	})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for wrong media_id, got %d", w.Code)
	}
}

func TestChaptersHandler_ParentalBlocked_NotFound(t *testing.T) {
	item := &domainlibrary.MediaItem{
		ID: "m1", LibraryID: "lib1",
		File:           &domainlibrary.MediaFile{Path: "/tmp/fake.mkv"},
		Classification: &domainlibrary.MediaClassification{OfficialRating: "R"},
	}
	h := chaptersHandler(
		&fakeChaptersGetter{},
		"http://localhost",
		allowAll(),
		&fakeMediaItemGetter{item: item},
		alwaysBlockChecker{},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/chapters", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 from parental gate (existence hidden), got %d", w.Code)
	}
}

func TestChaptersHandler_ParentalAdmin_OK(t *testing.T) {
	item := &domainlibrary.MediaItem{
		ID: "m1", LibraryID: "lib1",
		File:           &domainlibrary.MediaFile{Path: "/tmp/fake.mkv"},
		Classification: &domainlibrary.MediaClassification{OfficialRating: "R"},
	}
	h := chaptersHandler(
		&fakeChaptersGetter{},
		"http://localhost",
		allowAll(),
		&fakeMediaItemGetter{item: item},
		alwaysBlockChecker{},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/chapters", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(true))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("admin must bypass parental, got %d", w.Code)
	}
}

// ============================================================
// hlsSegmentHandler: no active session, falls back to disk.
// Empty cache dir -> 404 (segment not found). Exercises the
// no-session branch + safeJoin + os.Stat failure.
// ============================================================

// Post-fix: segments require a live session. Stale disk cache without a
// session must return 404 - the client re-fetches master.m3u8 which recreates
// the session with the caller's own decisions (closes cross-user leak after
// server restart).

// ============================================================
// hlsMasterHandler.serveHTTP - exercise the full happy path with
// fake HLSProcess + UserMaxStreams + bitrate + token injection.
// This drives serveMaster, GetOrCreate, and parseClientCapabilities.
// ============================================================

type fakeBitrateGetter struct {
	limit int
	err   error
}

func (f *fakeBitrateGetter) GetUserBitrateLimit(_ context.Context, _ string) (int, error) {
	return f.limit, f.err
}
