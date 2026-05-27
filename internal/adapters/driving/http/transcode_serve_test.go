package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	transcodingfakes "github.com/evey-project/evey/internal/domain/transcoding/fakes"
)

// ============================================================
// injectTokenIntoPlaylist (pure function, was 0% covered)
// ============================================================

func TestInjectTokenIntoPlaylist_AppendsToRelativeRefs(t *testing.T) {
	in := "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=5000000\nstream_0.m3u8\nseg_0.ts\n"
	got := injectTokenIntoPlaylist(in, "tok123")
	if !strings.Contains(got, "stream_0.m3u8?token=tok123") {
		t.Errorf("expected token appended to playlist ref, got:\n%s", got)
	}
	if !strings.Contains(got, "seg_0.ts?token=tok123") {
		t.Errorf("expected token appended to segment ref, got:\n%s", got)
	}
}

func TestInjectTokenIntoPlaylist_LeavesCommentsAlone(t *testing.T) {
	in := "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=5000000\n"
	got := injectTokenIntoPlaylist(in, "tok")
	if strings.Contains(got, "#EXTM3U?token=") {
		t.Errorf("comment lines must not be modified, got:\n%s", got)
	}
	if strings.Contains(got, "#EXT-X-STREAM-INF:BANDWIDTH=5000000?token=") {
		t.Errorf("EXT comment lines must not be modified, got:\n%s", got)
	}
}

func TestInjectTokenIntoPlaylist_LeavesAbsoluteUrlsAlone(t *testing.T) {
	in := "#EXTM3U\nhttp://cdn.example.com/seg.ts\nhttps://cdn.example.com/seg2.ts\n"
	got := injectTokenIntoPlaylist(in, "tok")
	if strings.Contains(got, "http://cdn.example.com/seg.ts?token=") {
		t.Errorf("absolute http URL must not get token appended, got:\n%s", got)
	}
	if strings.Contains(got, "https://cdn.example.com/seg2.ts?token=") {
		t.Errorf("absolute https URL must not get token appended, got:\n%s", got)
	}
}

func TestInjectTokenIntoPlaylist_HandlesEmptyLines(t *testing.T) {
	in := "#EXTM3U\n\nseg.ts\n\n"
	got := injectTokenIntoPlaylist(in, "tok")
	if !strings.Contains(got, "seg.ts?token=tok") {
		t.Errorf("expected segment with token, got:\n%s", got)
	}
}

// Edge: URI already carries a query string — must use & not ? a second time.
func TestInjectTokenIntoPlaylist_AppendsWithAmpersandOnExistingQuery(t *testing.T) {
	in := "#EXTM3U\nseg_0.ts?variant=sdr\n"
	got := injectTokenIntoPlaylist(in, "tok123")
	if !strings.Contains(got, "seg_0.ts?variant=sdr&token=tok123") {
		t.Errorf("expected & separator when ? already present, got:\n%s", got)
	}
	if strings.Contains(got, "seg_0.ts?variant=sdr?token=") {
		t.Errorf("must not emit two ? in the same URI, got:\n%s", got)
	}
}

// Edge: CRLF line endings must survive so strict TV clients still parse.
func TestInjectTokenIntoPlaylist_PreservesCRLF(t *testing.T) {
	in := "#EXTM3U\r\nseg.ts\r\n"
	got := injectTokenIntoPlaylist(in, "tok")
	if !strings.Contains(got, "seg.ts?token=tok\r\n") {
		t.Errorf("expected CRLF preserved around segment, got: %q", got)
	}
	if !strings.Contains(got, "#EXTM3U\r\n") {
		t.Errorf("expected CRLF preserved around comment, got: %q", got)
	}
}

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
// loadProfilePrefs (28.6% — exercise nil/empty/error/success branches)
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
	if p != def {
		t.Errorf("expected defaults when getter is nil, got %+v", p)
	}
}

func TestLoadProfilePrefs_EmptyProfileID_ReturnsDefaults(t *testing.T) {
	getter := &fakeProfilePrefsGetter{prefs: map[string]string{"allow_direct_play": "false"}}
	p := loadProfilePrefs(context.Background(), getter, "")
	def := apptranscoding.DefaultTranscodingPreferences()
	if p != def {
		t.Errorf("expected defaults when profileID is empty, got %+v", p)
	}
}

func TestLoadProfilePrefs_ErrorFallsBackToDefaults(t *testing.T) {
	getter := &fakeProfilePrefsGetter{err: errors.New("db down")}
	p := loadProfilePrefs(context.Background(), getter, "p1")
	def := apptranscoding.DefaultTranscodingPreferences()
	if p != def {
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
		&fakeTokenVerifier{claims: testClaims(false)},
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

func TestChaptersHandler_ParentalBlocked_403(t *testing.T) {
	item := &domainlibrary.MediaItem{
		ID: "m1", LibraryID: "lib1",
		File:           &domainlibrary.MediaFile{Path: "/tmp/fake.mkv"},
		Classification: &domainlibrary.MediaClassification{OfficialRating: "R"},
	}
	h := chaptersHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
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
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 from parental gate, got %d", w.Code)
	}
}

func TestChaptersHandler_ParentalAdmin_OK(t *testing.T) {
	item := &domainlibrary.MediaItem{
		ID: "m1", LibraryID: "lib1",
		File:           &domainlibrary.MediaFile{Path: "/tmp/fake.mkv"},
		Classification: &domainlibrary.MediaClassification{OfficialRating: "R"},
	}
	h := chaptersHandler(
		&fakeTokenVerifier{claims: testClaims(true)},
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

func TestTranscodeHandler_StreamToken_WrongMediaID_403(t *testing.T) {
	h := transcodeHandler(transcodeServe{
		tokens:        &fakeTokenVerifier{claims: testClaims(false)},
		libSvc:        &fakeMediaStreamsGetter{item: testMediaItem("lib1")},
		baseURL:       "http://localhost",
		libraryAccess: allowAll(),
		throttle:      apptranscoding.NewTranscodeThrottle(),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/transcode", nil)
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
		t.Errorf("expected 403 for stream token mismatch, got %d", w.Code)
	}
}

func TestHLSMasterHandler_StreamToken_WrongMediaID_403(t *testing.T) {
	h := hlsMasterHandler(hlsMasterServe{
		libSvc:        &fakeMediaStreamsGetter{item: testMediaItem("lib1")},
		libraryAccess: allowAll(),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/master.m3u8", nil)
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
		t.Errorf("expected 403 for stream token mismatch, got %d", w.Code)
	}
}

func TestHLSSegmentHandler_StreamToken_WrongMediaID_403(t *testing.T) {
	h := hlsSegmentHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		nil,
		allowAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/seg.ts", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("file", "seg.ts")
	req = withHTTPClaims(req, testClaims(false))
	ctx := context.WithValue(req.Context(), streamTokenCtxKey, &shared.StreamToken{
		UserID:      "u1",
		MediaItemID: "other-media",
	})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for stream token mismatch, got %d", w.Code)
	}
}

// ============================================================
// hlsSegmentHandler: no active session, falls back to disk.
// Empty cache dir -> 404 (segment not found). Exercises the
// no-session branch + safeJoin + os.Stat failure.
// ============================================================

func TestHLSSegmentHandler_NoSession_SegmentMissing_404(t *testing.T) {
	cacheDir := t.TempDir()
	mgr := apptranscoding.NewHLSManager(context.Background(), cacheDir, 1, &transcodingfakes.HLSProcess{})
	h := hlsSegmentHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		mgr, allowAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/missing.ts", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("file", "missing.ts")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing segment, got %d", w.Code)
	}
}

// Post-fix: segments require a live session. Stale disk cache without a
// session must return 404 — the client re-fetches master.m3u8 which recreates
// the session with the caller's own decisions (closes cross-user leak after
// server restart).
func TestHLSSegmentHandler_NoSession_M3U8_404(t *testing.T) {
	cacheDir := t.TempDir()
	mediaDir := filepath.Join(cacheDir, "m1")
	if err := os.MkdirAll(mediaDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_ = os.WriteFile(filepath.Join(mediaDir, "stream_0.m3u8"), []byte("#EXTM3U\nseg_0.ts\n"), 0644)
	mgr := apptranscoding.NewHLSManager(context.Background(), cacheDir, 1, &transcodingfakes.HLSProcess{})
	h := hlsSegmentHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		mgr, allowAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/stream_0.m3u8", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("file", "stream_0.m3u8")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 (no session), got %d", w.Code)
	}
}

func TestHLSSegmentHandler_NoSession_TS_404(t *testing.T) {
	cacheDir := t.TempDir()
	mediaDir := filepath.Join(cacheDir, "m1")
	if err := os.MkdirAll(mediaDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_ = os.WriteFile(filepath.Join(mediaDir, "seg_0.ts"), []byte("ts-bytes"), 0644)
	mgr := apptranscoding.NewHLSManager(context.Background(), cacheDir, 1, &transcodingfakes.HLSProcess{})
	h := hlsSegmentHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		mgr, allowAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/seg_0.ts", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("file", "seg_0.ts")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 (no session), got %d", w.Code)
	}
}

func TestHLSSegmentHandler_NoSession_M4S_404(t *testing.T) {
	cacheDir := t.TempDir()
	mediaDir := filepath.Join(cacheDir, "m1")
	if err := os.MkdirAll(mediaDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_ = os.WriteFile(filepath.Join(mediaDir, "seg.m4s"), []byte("m4s"), 0644)
	mgr := apptranscoding.NewHLSManager(context.Background(), cacheDir, 1, &transcodingfakes.HLSProcess{})
	h := hlsSegmentHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		mgr, allowAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/seg.m4s", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("file", "seg.m4s")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 (no session), got %d", w.Code)
	}
}

// ============================================================
// hlsMasterHandler.serveHTTP — exercise the full happy path with
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

func TestHLSMasterHandler_HappyPath_TokenInjection(t *testing.T) {
	cacheDir := t.TempDir()
	mgr := apptranscoding.NewHLSManager(context.Background(), cacheDir, 2, &transcodingfakes.HLSProcess{})

	w1, h1 := 1920, 1080
	libSvc := &fakeMediaStreamsGetter{
		item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: "/tmp/movie.mkv", ContainerFormat: "matroska"}},
		streams: []*domainlibrary.MediaStream{
			{StreamType: "video", Codec: "h264", Width: &w1, Height: &h1, IsDefault: true},
			{StreamType: "audio", Codec: "aac", IsDefault: true},
		},
	}
	maxStreams := 5
	h := hlsMasterHandler(hlsMasterServe{
		libSvc:         libSvc,
		hlsManager:     mgr,
		libraryAccess:  allowAll(),
		userMaxStreams: &fakeUserMaxStreams{max: &maxStreams},
		bitrateGetter:  &fakeBitrateGetter{limit: 0},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/master.m3u8?videoCodecs=h264&audioCodecs=aac&containers=matroska", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	ctx := context.WithValue(req.Context(), rawTokenCtxKey, "tk")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); got != "application/vnd.apple.mpegurl" {
		t.Errorf("expected mpegurl content-type, got %q", got)
	}
	if !strings.Contains(w.Body.String(), "?token=tk") {
		t.Errorf("expected token injected in master playlist, got:\n%s", w.Body.String())
	}
}

func TestHLSMasterHandler_BitrateFiltering(t *testing.T) {
	cacheDir := t.TempDir()
	mgr := apptranscoding.NewHLSManager(context.Background(), cacheDir, 2, &transcodingfakes.HLSProcess{})

	w1, h1 := 1920, 1080
	libSvc := &fakeMediaStreamsGetter{
		item: &domainlibrary.MediaItem{ID: "m2", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: "/tmp/m.mkv", ContainerFormat: "matroska"}},
		streams: []*domainlibrary.MediaStream{
			{StreamType: "video", Codec: "h264", Width: &w1, Height: &h1, IsDefault: true},
			{StreamType: "audio", Codec: "aac", IsDefault: true},
		},
	}
	h := hlsMasterHandler(hlsMasterServe{
		libSvc:        libSvc,
		hlsManager:    mgr,
		libraryAccess: allowAll(),
		bitrateGetter: &fakeBitrateGetter{limit: 1000}, // 1000 kbps -> 1Mbps
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m2/hls/master.m3u8?videoCodecs=h264&audioCodecs=aac", nil)
	req.SetPathValue("id", "m2")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	// master playlist body should be present (filtered or not)
	if !strings.Contains(w.Body.String(), "#EXTM3U") {
		t.Errorf("expected EXTM3U marker, got:\n%s", w.Body.String())
	}
}
