package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	appfeed "github.com/gravity-zero/evey/internal/app/feed"
	"github.com/gravity-zero/evey/internal/domain/feed"
	feedfakes "github.com/gravity-zero/evey/internal/domain/feed/fakes"
	"github.com/gravity-zero/evey/internal/pkg/auth"
)

func newFeedCmdService() *appfeed.CommandService {
	return appfeed.NewCommandService(
		feedfakes.NewFakeConfigCommandRepo(),
		feedfakes.NewFakeConfigQueryRepo(),
	)
}

func newFeedQueryService() *appfeed.QueryService {
	return appfeed.NewQueryService(
		feedfakes.NewFakeConfigQueryRepo(),
		feedfakes.NewFakeFeedQuerier(),
		nil,
	)
}

func newFeedQueryServiceWithItems(items []*feed.FeedItem) *appfeed.QueryService {
	q := feedfakes.NewFakeFeedQuerier()
	q.ResumePlaybackItems = items
	q.NextEpisodeItems = items
	return appfeed.NewQueryService(
		feedfakes.NewFakeConfigQueryRepo(),
		q,
		nil,
	)
}

func feedMux(t *testing.T, tokenGen *auth.JWTGenerator, querySvc *appfeed.QueryService) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	registerFeedRoutes(mux, newFeedCmdService(), querySvc, tokenGen)
	return mux
}

func TestResumePlayback_NoAuth_401(t *testing.T) {
	gen, _, _ := genTokens(t)
	mux := feedMux(t, gen, newFeedQueryService())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/feeds/resume-playback", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestResumePlayback_InvalidToken_401(t *testing.T) {
	gen, _, _ := genTokens(t)
	mux := feedMux(t, gen, newFeedQueryService())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/feeds/resume-playback", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestResumePlayback_OK(t *testing.T) {
	gen, _, userToken := genTokens(t)
	items := []*feed.FeedItem{
		{MediaItemID: "m1", Title: "Test Movie", Type: "movie", PositionMs: 5000},
	}
	mux := feedMux(t, gen, newFeedQueryServiceWithItems(items))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/feeds/resume-playback", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if _, ok := resp["items"]; !ok {
		t.Error("expected 'items' key in response")
	}
}

func TestNextEpisode_NoAuth_401(t *testing.T) {
	gen, _, _ := genTokens(t)
	mux := feedMux(t, gen, newFeedQueryService())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/feeds/next-episode", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestNextEpisode_OK(t *testing.T) {
	gen, _, userToken := genTokens(t)
	items := []*feed.FeedItem{
		{MediaItemID: "m2", Title: "Episode 2", Type: "episode"},
	}
	mux := feedMux(t, gen, newFeedQueryServiceWithItems(items))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/feeds/next-episode", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
