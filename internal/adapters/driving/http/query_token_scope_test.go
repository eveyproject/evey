package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appbackup "github.com/evey-project/evey/internal/app/backup"
	appmedia "github.com/evey-project/evey/internal/app/media"
	backupfakes "github.com/evey-project/evey/internal/domain/backup/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/auth"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
)

// An account JWT that leaks through a proxy log or a browser history must not
// be able to write, nor reach the admin surface: ?token= is promoted to a
// Bearer header for reads outside /api/v1/admin/ only, and it leaves the URL
// in every case.
func TestTokenRedact_AccountJWTInQueryIsForReadsOutsideAdmin(t *testing.T) {
	cases := []struct {
		method, path string
		promoted     bool
	}{
		{http.MethodGet, "/api/v1/music/tracks/t1/art", true},
		{http.MethodHead, "/api/v1/photos/p1", true},
		{http.MethodPost, "/api/v1/users", false},
		{http.MethodDelete, "/api/v1/playlists/p1", false},
		{http.MethodGet, "/api/v1/admin/backups/bk-1", false},
		{http.MethodGet, "/api/v1/admin/logs/search", false},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			var gotAuth, gotQuery string
			next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				gotAuth = r.Header.Get("Authorization")
				gotQuery = r.URL.Query().Get("token")
			})
			req := httptest.NewRequest(tc.method, tc.path+"?token=head.body.sig", nil)
			tokenRedactMiddleware(nil, false, next).ServeHTTP(httptest.NewRecorder(), req)
			if gotQuery != "" {
				t.Fatal("the token must leave the URL")
			}
			if got := gotAuth == "Bearer head.body.sig"; got != tc.promoted {
				t.Fatalf("promoted = %v, want %v (Authorization %q)", got, tc.promoted, gotAuth)
			}
		})
	}
}

func newBackupTokenServer(t *testing.T) (http.Handler, string, string) {
	t.Helper()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	adminJWT, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", IsAdmin: true})
	userJWT, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "u2"})
	svc := appbackup.NewService(backupfakes.NewFakeBackupStore(), backupfakes.NewFakeDatabaseDumper(), "1.0.0")
	tokens := appmedia.NewStreamTokenStore(time.Minute)
	mux := http.NewServeMux()
	registerBackupRoutes(mux, svc, gen, tokens, ratelimit.New(100), ratelimit.New(100))
	return TokenRedactMiddlewareWithStore(tokens, false, mux), adminJWT, userJWT
}

func doReq(t *testing.T, h http.Handler, method, url, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, url, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// A backup is downloaded by URL (an anchor, a native download) with a short
// token bound to that one backup, minted by an admin over a header - never
// with the account JWT in the URL.
func TestBackupDownload_TokenBoundToOneBackup(t *testing.T) {
	h, adminJWT, userJWT := newBackupTokenServer(t)

	created := doReq(t, h, http.MethodPost, "/api/v1/admin/backups", adminJWT)
	if created.Code != http.StatusCreated && created.Code != http.StatusOK {
		t.Fatalf("create backup: %d %s", created.Code, created.Body)
	}
	var m struct{ ID string }
	if err := json.Unmarshal(created.Body.Bytes(), &m); err != nil || m.ID == "" {
		t.Fatalf("create backup response %s: %v", created.Body, err)
	}

	if w := doReq(t, h, http.MethodPost, "/api/v1/admin/backups/"+m.ID+"/download-token", userJWT); w.Code != http.StatusForbidden {
		t.Fatalf("a non-admin must not mint a backup token, got %d", w.Code)
	}
	minted := doReq(t, h, http.MethodPost, "/api/v1/admin/backups/"+m.ID+"/download-token", adminJWT)
	var tok struct{ Token string }
	if err := json.Unmarshal(minted.Body.Bytes(), &tok); err != nil || tok.Token == "" {
		t.Fatalf("mint: %d %s", minted.Code, minted.Body)
	}

	if w := doReq(t, h, http.MethodGet, "/api/v1/admin/backups/"+m.ID+"?token="+tok.Token, ""); w.Code != http.StatusOK {
		t.Fatalf("download with the bound token: %d %s", w.Code, w.Body)
	}
	if w := doReq(t, h, http.MethodGet, "/api/v1/admin/backups/bk-other?token="+tok.Token, ""); w.Code != http.StatusForbidden {
		t.Fatalf("a token bound to another backup must be refused, got %d", w.Code)
	}
	if w := doReq(t, h, http.MethodGet, "/api/v1/admin/backups/"+m.ID+"?token="+adminJWT, ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("the admin JWT in the URL must no longer download, got %d", w.Code)
	}
}

// A backup token is not a media capability: the streaming routes refuse it.
func TestStreamAuth_RefusesABackupToken(t *testing.T) {
	reached := false
	h := httpStreamAuthMiddleware(nil)(func(http.ResponseWriter, *http.Request) { reached = true })
	st := &shared.StreamToken{UserID: "u1", MediaItemID: "bk-1", Scope: shared.ScopeBackupDownload}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/bk-1/stream", nil)
	req = req.WithContext(context.WithValue(req.Context(), streamTokenCtxKey, st))
	w := httptest.NewRecorder()
	h(w, req)
	if reached || w.Code != http.StatusForbidden {
		t.Fatalf("backup token on a media route: reached=%v code=%d", reached, w.Code)
	}
}
