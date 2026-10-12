package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appmedia "github.com/eveyproject/evey/internal/app/media"
	"github.com/eveyproject/evey/internal/domain/shared"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
)

// The account JWT used to ride in ZIP-download URLs (collections, playlists,
// music) - reverse-proxy logs and browser history then hold a replayable
// session credential. The mint endpoint trades the JWT (sent once, in a
// header) for a short-lived opaque token bound to ONE resource id; the
// download handlers then enforce that binding.

func mintReq(id string, claims *domainuser.Claims) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/download-token/"+id, nil)
	r.SetPathValue("id", id)
	if claims != nil {
		r = r.WithContext(context.WithValue(r.Context(), httpClaimsCtxKey{}, claims))
	}
	return r
}

func TestDownloadTokenHandler_MintsABoundToken(t *testing.T) {
	store := appmedia.NewStreamTokenStore(appmedia.DefaultStreamTokenTTL)
	defer store.Stop()
	h := downloadTokenHandler(store)

	rec := httptest.NewRecorder()
	h(rec, mintReq("col-1", &domainuser.Claims{UserID: "u1", ProfileID: "p1"}))
	if rec.Code != http.StatusOK {
		t.Fatalf("mint = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	st, ok := store.Validate(resp["token"], "col-1")
	if !ok {
		t.Fatal("minted token must validate for its resource id")
	}
	if st.UserID != "u1" || st.ProfileID != "p1" {
		t.Fatalf("token must carry the minting identity, got %q/%q", st.UserID, st.ProfileID)
	}
	if _, ok := store.Validate(resp["token"], "col-OTHER"); ok {
		t.Fatal("token must NOT validate for another resource id")
	}
}

func TestDownloadTokenHandler_RejectsUnusableIDs(t *testing.T) {
	store := appmedia.NewStreamTokenStore(appmedia.DefaultStreamTokenTTL)
	defer store.Stop()
	h := downloadTokenHandler(store)

	for _, id := range []string{"", strings.Repeat("a", 200)} {
		rec := httptest.NewRecorder()
		h(rec, mintReq(id, &domainuser.Claims{UserID: "u1"}))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("id %q: want 400, got %d", id, rec.Code)
		}
	}
}

// requireDownloadTokenBinding is the per-handler guard: a store token must be
// bound to THIS resource (a media-scoped stream token must not download an
// unrelated collection), and a syncplay guest may stream but never download.
// A JWT-authenticated request (no store token in ctx) passes untouched.
func TestRequireDownloadTokenBinding(t *testing.T) {
	withTok := func(st *shared.StreamToken) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		if st == nil {
			return r
		}
		return r.WithContext(context.WithValue(r.Context(), streamTokenCtxKey, st))
	}

	cases := []struct {
		name string
		st   *shared.StreamToken
		id   string
		want bool
	}{
		{"jwt path (no token)", nil, "col-1", true},
		{"bound to this resource", &shared.StreamToken{MediaItemID: "col-1", Scope: shared.ScopeStream}, "col-1", true},
		{"bound to another resource", &shared.StreamToken{MediaItemID: "mi-9", Scope: shared.ScopeStream}, "col-1", false},
		{"syncplay guest", &shared.StreamToken{MediaItemID: "col-1", Scope: shared.ScopeSyncplayGuest}, "col-1", false},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		got := requireDownloadTokenBinding(rec, withTok(c.st), c.id)
		if got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
		if !c.want && rec.Code != http.StatusForbidden {
			t.Errorf("%s: refusal must write 403, got %d", c.name, rec.Code)
		}
	}
}
