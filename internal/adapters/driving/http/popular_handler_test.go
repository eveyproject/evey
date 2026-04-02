package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	appactivity "github.com/gravity-zero/evey/internal/app/activity"
	applibrary "github.com/gravity-zero/evey/internal/app/library"
	"github.com/gravity-zero/evey/internal/domain/activity"
	activityfakes "github.com/gravity-zero/evey/internal/domain/activity/fakes"
	"github.com/gravity-zero/evey/internal/domain/library"
	libfakes "github.com/gravity-zero/evey/internal/domain/library/fakes"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	userfakes "github.com/gravity-zero/evey/internal/domain/user/fakes"
)

type popularTestFixture struct {
	mux        *http.ServeMux
	token      string
	mediaRepo  *libfakes.FakeMediaItemQueryRepo
	accessRepo *userfakes.FakeLibraryAccessRepository
	statsReader *activityfakes.FakePlaySessionStatsReader
}

func newPopularTestFixture(t *testing.T, claims domainuser.Claims) *popularTestFixture {
	t.Helper()
	tokenGen := newTestJWTGen(t)

	statsReader := activityfakes.NewFakePlaySessionStatsReader()
	activitySvc := appactivity.NewQueryService(
		activityfakes.NewFakeLoginHistoryRepository(),
		activityfakes.NewFakeServerMetricRepository(),
		activityfakes.NewFakeAuditLogRepository(),
		statsReader,
		activityfakes.NewFakePlayHistoryRepository(),
		appactivity.WithAnalyticsReader(activityfakes.NewFakeAnalyticsReader()),
	)

	mediaRepo := libfakes.NewFakeMediaItemQueryRepo()
	libQuery := applibrary.NewQueryService(
		libfakes.NewFakeLibraryQueryRepo(),
		mediaRepo,
		libfakes.NewFakeMediaStreamRepository(),
		libfakes.NewFakeSearchIndex(),
	)

	accessRepo := userfakes.NewFakeLibraryAccessRepository()

	tok, err := tokenGen.GenerateAccessToken(claims)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	mux := http.NewServeMux()
	registerPopularRoute(mux, activitySvc, libQuery, tokenGen, accessRepo)
	return &popularTestFixture{
		mux: mux, token: tok, mediaRepo: mediaRepo,
		accessRepo: accessRepo, statsReader: statsReader,
	}
}

func newPopularTestMux(t *testing.T) (*http.ServeMux, string) {
	t.Helper()
	f := newPopularTestFixture(t, domainuser.Claims{UserID: "u1", IsAdmin: false})
	return f.mux, f.token
}

func TestPopularRequiresAuth(t *testing.T) {
	tv := invalidValidator()
	handler := requireAdmin(tv, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest("GET", "/api/v1/popular", nil)
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestPopular_NoAuth_401(t *testing.T) {
	mux, _ := newPopularTestMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/popular", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestPopular_ValidAuth_200(t *testing.T) {
	mux, tok := newPopularTestMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/popular", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestPopular_IDOR_FiltersInaccessible(t *testing.T) {
	f := newPopularTestFixture(t, domainuser.Claims{UserID: "u1", IsAdmin: false})
	f.mediaRepo.Seed(&library.MediaItem{ID: "m1", LibraryID: "lib1", Title: "Visible Movie"})
	f.mediaRepo.Seed(&library.MediaItem{ID: "m2", LibraryID: "lib-secret", Title: "Secret Movie"})
	f.statsReader.MostPopular = []activity.MediaPlayCount{
		{MediaItemID: "m1", Title: "Visible Movie", PlayCount: 10},
		{MediaItemID: "m2", Title: "Secret Movie", PlayCount: 5},
	}
	// restrict user: only lib1
	_ = f.accessRepo.Grant(context.Background(), "u1", "lib1")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/popular", nil)
	req.Header.Set("Authorization", "Bearer "+f.token)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Items []struct {
			MediaItemID string `json:"mediaItemId"`
		} `json:"items"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Items))
	}
	if resp.Items[0].MediaItemID != "m1" {
		t.Errorf("expected m1, got %s", resp.Items[0].MediaItemID)
	}
}

func TestPopular_Admin_SeesAll(t *testing.T) {
	f := newPopularTestFixture(t, domainuser.Claims{UserID: "admin1", IsAdmin: true})
	f.mediaRepo.Seed(&library.MediaItem{ID: "m1", LibraryID: "lib1", Title: "Movie A"})
	f.mediaRepo.Seed(&library.MediaItem{ID: "m2", LibraryID: "lib-secret", Title: "Movie B"})
	f.statsReader.MostPopular = []activity.MediaPlayCount{
		{MediaItemID: "m1", Title: "Movie A", PlayCount: 10},
		{MediaItemID: "m2", Title: "Movie B", PlayCount: 5},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/popular", nil)
	req.Header.Set("Authorization", "Bearer "+f.token)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Items []struct {
			MediaItemID string `json:"mediaItemId"`
		} `json:"items"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(resp.Items))
	}
}
