package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	domainlibrary "github.com/gravity-zero/evey/internal/domain/library"
)

// fakeMediaItemLister is a test double for mediaItemLister.
type fakeMediaItemLister struct {
	items []*domainlibrary.MediaItem
	err   error
}

func (f *fakeMediaItemLister) ListMediaItems(_ context.Context, filter domainlibrary.ListMediaItemsFilter) ([]*domainlibrary.MediaItem, error) {
	if f.err != nil {
		return nil, f.err
	}
	var out []*domainlibrary.MediaItem
	for _, item := range f.items {
		if filter.YearFrom != 0 && (item.Year == nil || *item.Year < filter.YearFrom) {
			continue
		}
		if filter.YearTo != 0 && (item.Year == nil || *item.Year > filter.YearTo) {
			continue
		}
		if filter.MinRating != 0 && (item.CommunityRating == nil || *item.CommunityRating < filter.MinRating) {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

func (f *fakeMediaItemLister) CountMediaItems(_ context.Context, filter domainlibrary.ListMediaItemsFilter) (int, error) {
	if f.err != nil {
		return 0, f.err
	}
	items, _ := f.ListMediaItems(context.Background(), filter)
	return len(items), nil
}

func newFilterTestServer(lister mediaItemLister, tokenVerifier *fakeTokenVerifier, checker LibraryAccessChecker) http.Handler {
	mux := http.NewServeMux()
	registerFilterRoutes(mux, lister, tokenVerifier, checker)
	return mux
}

func TestFilterHandler_Unauthenticated(t *testing.T) {
	mux := newFilterTestServer(&fakeMediaItemLister{}, &fakeTokenVerifier{err: errTest}, allowAll())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/filter", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestFilterHandler_YearFrom(t *testing.T) {
	year2010 := 2010
	year2020 := 2020
	lister := &fakeMediaItemLister{
		items: []*domainlibrary.MediaItem{
			{ID: "old", Title: "Old", Year: &year2010},
			{ID: "new", Title: "New", Year: &year2020},
		},
	}
	mux := newFilterTestServer(lister, &fakeTokenVerifier{claims: testClaims(true)}, allowAll())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/filter?year_from=2015", nil)
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Items []struct{ ID string } `json:"items"`
		Total int                   `json:"total"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 1 {
		t.Errorf("expected total=1, got %d", resp.Total)
	}
	if len(resp.Items) != 1 || resp.Items[0].ID != "new" {
		t.Errorf("expected only 'new' item, got %+v", resp.Items)
	}
}

func TestFilterHandler_YearTo(t *testing.T) {
	year2010 := 2010
	year2020 := 2020
	lister := &fakeMediaItemLister{
		items: []*domainlibrary.MediaItem{
			{ID: "old", Title: "Old", Year: &year2010},
			{ID: "new", Title: "New", Year: &year2020},
		},
	}
	mux := newFilterTestServer(lister, &fakeTokenVerifier{claims: testClaims(true)}, allowAll())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/filter?year_to=2015", nil)
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Items []struct{ ID string } `json:"items"`
		Total int                   `json:"total"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 1 || resp.Items[0].ID != "old" {
		t.Errorf("expected only 'old' item, got total=%d items=%+v", resp.Total, resp.Items)
	}
}

func TestFilterHandler_MinRating(t *testing.T) {
	r8 := 8.0
	r5 := 5.0
	lister := &fakeMediaItemLister{
		items: []*domainlibrary.MediaItem{
			{ID: "high", Title: "High", CommunityRating: &r8},
			{ID: "low", Title: "Low", CommunityRating: &r5},
		},
	}
	mux := newFilterTestServer(lister, &fakeTokenVerifier{claims: testClaims(true)}, allowAll())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/filter?min_rating=7.0", nil)
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Items []struct{ ID string } `json:"items"`
		Total int                   `json:"total"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 1 || resp.Items[0].ID != "high" {
		t.Errorf("expected only 'high' item, got total=%d items=%+v", resp.Total, resp.Items)
	}
}

func TestFilterHandler_YearRange(t *testing.T) {
	y2000 := 2000
	y2010 := 2010
	y2020 := 2020
	lister := &fakeMediaItemLister{
		items: []*domainlibrary.MediaItem{
			{ID: "a", Year: &y2000},
			{ID: "b", Year: &y2010},
			{ID: "c", Year: &y2020},
		},
	}
	mux := newFilterTestServer(lister, &fakeTokenVerifier{claims: testClaims(true)}, allowAll())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/filter?year_from=2005&year_to=2015", nil)
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Items []struct{ ID string } `json:"items"`
		Total int                   `json:"total"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 1 || resp.Items[0].ID != "b" {
		t.Errorf("expected only 'b', got total=%d items=%+v", resp.Total, resp.Items)
	}
}

func TestFilterHandler_LibraryAccessDenied(t *testing.T) {
	lister := &fakeMediaItemLister{}
	mux := newFilterTestServer(lister, &fakeTokenVerifier{claims: testClaims(false)}, denyAll())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/filter?library_id=lib-1", nil)
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}
