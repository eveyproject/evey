package httpapi

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appfeed "github.com/gravity-zero/evey/internal/app/feed"
	"github.com/gravity-zero/evey/internal/domain/feed"
	feedfakes "github.com/gravity-zero/evey/internal/domain/feed/fakes"
	notifakes "github.com/gravity-zero/evey/internal/domain/notification/fakes"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/auth"
)

func newFeedTestMux(t *testing.T, querier *feedfakes.FakeFeedQuerier) (*http.ServeMux, string, string) {
	t.Helper()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	adminToken, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "admin-1", IsAdmin: true, ProfileID: "prof-admin"})
	userToken, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", IsAdmin: false, ProfileID: "prof-1"})

	cmdRepo := feedfakes.NewFakeConfigCommandRepo()
	queryRepo := feedfakes.NewFakeConfigQueryRepo()
	cmdSvc := appfeed.NewCommandService(cmdRepo, queryRepo)
	querySvc := appfeed.NewQueryService(queryRepo, querier, notifakes.NewFakeWatchHistoryAnalyzer())

	mux := http.NewServeMux()
	registerFeedRoutes(mux, cmdSvc, querySvc, gen)
	return mux, adminToken, userToken
}

// --- batch update feed config ---

func TestBatchUpdateFeedConfig_NoAuth_401(t *testing.T) {
	mux, _, _ := newFeedTestMux(t, feedfakes.NewFakeFeedQuerier())
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/feeds/config", strings.NewReader(`[]`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestBatchUpdateFeedConfig_NonAdmin_403(t *testing.T) {
	mux, _, userToken := newFeedTestMux(t, feedfakes.NewFakeFeedQuerier())
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/feeds/config", strings.NewReader(`[]`))
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestBatchUpdateFeedConfig_Success_204(t *testing.T) {
	mux, adminToken, _ := newFeedTestMux(t, feedfakes.NewFakeFeedQuerier())
	body := `[{"feedType":"recently_added","enabled":true,"displayOrder":1,"maxItems":20}]`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/feeds/config", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestBatchUpdateFeedConfig_InvalidBody_400(t *testing.T) {
	mux, adminToken, _ := newFeedTestMux(t, feedfakes.NewFakeFeedQuerier())
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/feeds/config", strings.NewReader(`not json`))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestBatchUpdateFeedConfig_InvalidFeedType_400(t *testing.T) {
	mux, adminToken, _ := newFeedTestMux(t, feedfakes.NewFakeFeedQuerier())
	body := `[{"feedType":"","enabled":true,"displayOrder":1,"maxItems":20}]`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/feeds/config", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- resume playback ---

func TestResumePlayback_NoAuth_401(t *testing.T) {
	mux, _, _ := newFeedTestMux(t, feedfakes.NewFakeFeedQuerier())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/feeds/resume-playback", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestResumePlayback_InvalidToken_401(t *testing.T) {
	mux, _, _ := newFeedTestMux(t, feedfakes.NewFakeFeedQuerier())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/feeds/resume-playback", nil)
	req.Header.Set("Authorization", "Bearer invalid-garbage")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestResumePlayback_Success_200(t *testing.T) {
	q := feedfakes.NewFakeFeedQuerier()
	q.ResumePlaybackItems = []*feed.FeedItem{
		{MediaItemID: "m1", Title: "Movie 1", Type: "movie", PositionMs: 5000},
	}
	mux, _, userToken := newFeedTestMux(t, q)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/feeds/resume-playback", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Items []feedItemJSON `json:"items"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Items) != 1 {
		t.Errorf("expected 1 item, got %d", len(resp.Items))
	}
	if resp.Items[0].MediaItemID != "m1" {
		t.Errorf("expected mediaItemId 'm1', got %q", resp.Items[0].MediaItemID)
	}
}

func TestResumePlayback_WithLimit(t *testing.T) {
	q := feedfakes.NewFakeFeedQuerier()
	mux, _, userToken := newFeedTestMux(t, q)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/feeds/resume-playback?limit=5", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestResumePlayback_ServiceError_500(t *testing.T) {
	q := feedfakes.NewFakeFeedQuerier()
	q.Err = errTest
	mux, _, userToken := newFeedTestMux(t, q)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/feeds/resume-playback", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- next episode ---

func TestNextEpisode_NoAuth_401(t *testing.T) {
	mux, _, _ := newFeedTestMux(t, feedfakes.NewFakeFeedQuerier())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/feeds/next-episode", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestNextEpisode_Success_200(t *testing.T) {
	q := feedfakes.NewFakeFeedQuerier()
	q.NextEpisodeItems = []*feed.FeedItem{
		{MediaItemID: "ep2", ParentID: "series1", Title: "Episode 2", Type: "episode"},
	}
	mux, _, userToken := newFeedTestMux(t, q)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/feeds/next-episode", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Items []feedItemJSON `json:"items"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Items) != 1 {
		t.Errorf("expected 1 item, got %d", len(resp.Items))
	}
}

func TestNextEpisode_ServiceError_500(t *testing.T) {
	q := feedfakes.NewFakeFeedQuerier()
	q.Err = errTest
	mux, _, userToken := newFeedTestMux(t, q)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/feeds/next-episode", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- registerFeedRoutes with nil command service ---

func TestRegisterFeedRoutes_NilCmdService(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	mux := http.NewServeMux()
	registerFeedRoutes(mux, nil, nil, gen)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/feeds/resume-playback", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unregistered route, got %d", w.Code)
	}
}

// --- parseLimit ---

func TestParseLimit(t *testing.T) {
	tests := []struct {
		query    string
		defLimit int
		want     int
	}{
		{"", 20, 20},
		{"limit=10", 20, 10},
		{"limit=0", 20, 20},
		{"limit=-1", 20, 20},
		{"limit=200", 20, 20},
		{"limit=abc", 20, 20},
		{"limit=50", 20, 50},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, "/test?"+tt.query, nil)
		got := parseLimit(req, tt.defLimit)
		if got != tt.want {
			t.Errorf("parseLimit(%q, %d) = %d, want %d", tt.query, tt.defLimit, got, tt.want)
		}
	}
}
