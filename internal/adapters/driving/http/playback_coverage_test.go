package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	"github.com/eveyproject/evey/internal/config"
	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	domaintranscoding "github.com/eveyproject/evey/internal/domain/transcoding"
	"github.com/eveyproject/evey/internal/pkg/ratelimit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- shared helpers ---

// --- playbackInfoHandler ---

func TestPlaybackInfo_NoAuth_401(t *testing.T) {
	verifier := &fakeTokenVerifier{err: errTest}
	h := httpAuthMiddleware(verifier)(playbackInfoHandler(nil,
		&fakeMediaStreamsGetter{},
		allowAll(),
		nil, nil))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/playback-info", nil)
	req.SetPathValue("id", "m1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestPlaybackInfo_InvalidToken_401(t *testing.T) {
	verifier := &fakeTokenVerifier{err: errTest}
	h := httpAuthMiddleware(verifier)(playbackInfoHandler(nil,
		&fakeMediaStreamsGetter{},
		allowAll(),
		nil, nil))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/playback-info", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer bad")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestPlaybackInfo_NotFound_404(t *testing.T) {
	h := playbackInfoHandler(nil,
		&fakeMediaStreamsGetter{itemErr: errTest},
		allowAll(),
		nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/playback-info", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestPlaybackInfo_IDOR_403(t *testing.T) {
	h := playbackInfoHandler(nil,
		&fakeMediaStreamsGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib-x", File: &domainlibrary.MediaFile{Path: "/a"}}},
		denyAll(),
		nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/playback-info", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// A title the profile's parental rules hide answers like one that does not
// exist, even to a profile with access to its library: the playback decision
// names codecs, tracks and a stream URL.
func TestPlaybackInfo_ParentalBlocked_404(t *testing.T) {
	h := playbackInfoHandler(nil,
		&fakeMediaStreamsGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: "/a"}}},
		allowAll(),
		nil, testFakeParentalChecker{block: true})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/playback-info", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestPlaybackInfo_OK(t *testing.T) {
	w1 := 1920
	h1 := 1080
	h := playbackInfoHandler(nil,
		&fakeMediaStreamsGetter{
			item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: "/a.mkv", ContainerFormat: "matroska"}},
			streams: []*domainlibrary.MediaStream{
				{StreamType: "video", Codec: "h264", Width: &w1, Height: &h1},
				{StreamType: "audio", Codec: "aac", IsDefault: true},
			},
		},
		allowAll(),
		nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/playback-info", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// --- playback settings ---

func newPlaybackSettingsMux(t *testing.T) (*http.ServeMux, string, string) {
	t.Helper()
	gen, adminTok, userTok := genTokens(t)
	cfg := &config.Config{Playback: config.PlaybackConfig{SkipForwardSeconds: 30, SkipBackwardSeconds: 10}}
	mux := http.NewServeMux()
	registerPlaybackSettingsRoutes(mux, cfg, gen, ratelimit.New(100), nil)
	return mux, adminTok, userTok
}

func TestPlaybackSettings_Get_NoAuth_401(t *testing.T) {
	mux, _, _ := newPlaybackSettingsMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/playback-settings", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestPlaybackSettings_Get_Auth_200(t *testing.T) {
	mux, _, userTok := newPlaybackSettingsMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/playback-settings", nil)
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "skipForwardSeconds") {
		t.Error("expected skipForwardSeconds in response")
	}
}

func TestPlaybackSettings_Put_Admin_OK(t *testing.T) {
	mux, adminTok, _ := newPlaybackSettingsMux(t)
	body := `{"skipForwardSeconds":15}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/playback-settings", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestPlaybackSettings_Put_NonAdmin_403(t *testing.T) {
	mux, _, userTok := newPlaybackSettingsMux(t)
	body := `{"skipForwardSeconds":15}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/playback-settings", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

type fakeBitrateSvc struct{}

func (f *fakeBitrateSvc) GetUserBitrateLimit(_ context.Context, _ string) (int, error) {
	return 0, nil
}

func TestNewUserBitrateAdapter(t *testing.T) {
	adapter := NewUserBitrateAdapter(&fakeBitrateSvc{})
	limit, err := adapter.GetUserBitrateLimit(context.Background(), "nonexistent")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if limit != 0 {
		t.Errorf("expected 0 for missing user, got %d", limit)
	}
}

// recordingDecider keeps the request it was handed and answers a decision the
// core would not (a transcode on an all-copy source).
type recordingDecider struct {
	got      apptranscoding.PlaybackRequest
	calls    int
	decision domaintranscoding.PlaybackDecision
}

func (d *recordingDecider) Reported(_ context.Context, req apptranscoding.PlaybackRequest) domaintranscoding.PlaybackDecision {
	d.got = req
	d.calls++
	return d.decision
}

// playback-info answers with the bound decider's decision (ADR-0030: the
// plan decides, the info surface projects) and hands it the request the
// HLS master will be planned from: the client's codecs, the line cap, the
// selected subtitle, the source facts.
func TestPlaybackInfo_DecidesThroughTheBoundDecider(t *testing.T) {
	dec := &recordingDecider{decision: domaintranscoding.PlaybackDecision{
		PlayMethod: "transcode", VideoDecision: "transcode", AudioDecision: "copy",
		SubtitleDecision: "burn", VideoCodec: "h264", MaxHeight: 720, TranscodeReasons: []string{"bound_decider"},
	}}
	w1, h1 := 1920, 1080
	h := playbackInfoHandler(dec,
		&fakeMediaStreamsGetter{
			item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: "/srv/a.mp4", ContainerFormat: "mp4", Size: 1 << 30, Bitrate: 8_000_000}},
			streams: []*domainlibrary.MediaStream{
				{StreamIndex: 0, StreamType: "video", Codec: "h264", Width: &w1, Height: &h1},
				{StreamIndex: 1, StreamType: "audio", Codec: "aac", IsDefault: true},
				{StreamIndex: 2, StreamType: "subtitle", Codec: "hdmv_pgs_subtitle"},
			},
		},
		allowAll(), nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/playback-info?videoCodecs=hevc&maxBitrate=4000&subtitleTrack=0", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var body playbackInfoDTO
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, 1, dec.calls)
	assert.Equal(t, "transcode", body.PlayMethod)
	assert.Equal(t, "transcode", body.VideoDecision)
	assert.Equal(t, "copy", body.AudioDecision)
	assert.Equal(t, "burn", body.SubtitleDecision)
	assert.Equal(t, 720, body.MaxHeight)
	assert.Equal(t, []string{"bound_decider"}, body.TranscodeReasons)

	got := dec.got
	assert.Equal(t, "m1", got.MediaID)
	assert.Equal(t, "/srv/a.mp4", got.SourcePath)
	assert.Equal(t, "mp4", got.Container)
	assert.Len(t, got.Streams, 3)
	assert.Equal(t, []string{"hevc"}, got.Client.VideoCodecs)
	assert.Equal(t, int64(0), got.Client.MaxBitrate, "the line cap moves to the request")
	assert.Equal(t, 4000, got.MaxBitrateKbps)
	assert.Equal(t, 0, got.SubtitleBurnInTrack)
	assert.Equal(t, 0, got.Prefs.SubtitleBurnInTrack)
	assert.Equal(t, int64(1<<30), got.SourceBytes)
	assert.Equal(t, int64(8000), got.SourceBitrateKbps)
	assert.False(t, got.SourceNeedsRemux)
}

// A decider that forbids what the profile forbids: the 422 is decided on the
// decider's verdict, not re-derived.
func TestPlaybackInfo_DeciderTranscodeAgainstPrefs_422(t *testing.T) {
	dec := &recordingDecider{decision: domaintranscoding.PlaybackDecision{PlayMethod: "transcode", VideoDecision: "transcode"}}
	h := playbackInfoHandler(dec,
		&fakeMediaStreamsGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: "/srv/a.mp4", ContainerFormat: "mp4"}}},
		allowAll(), &fakeProfilePrefsGetter{prefs: map[string]string{"allow_transcoding": "false"}}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/playback-info", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
}
