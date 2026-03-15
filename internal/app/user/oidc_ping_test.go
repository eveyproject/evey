package user_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appuser "github.com/gravity-zero/evey/internal/app/user"
	"github.com/gravity-zero/evey/internal/config"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	userfakes "github.com/gravity-zero/evey/internal/domain/user/fakes"
)

func newTestOIDCServer(t *testing.T, issuer string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		doc := map[string]any{
			"issuer":                 issuer,
			"authorization_endpoint": issuer + "/auth",
			"token_endpoint":         issuer + "/token",
			"jwks_uri":               issuer + "/jwks",
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(doc); err != nil {
			http.Error(w, "encode error", http.StatusInternalServerError)
		}
	}))
	return srv
}

func newOIDCSvc(t *testing.T, issuerURL, clientSecret string) (*appuser.OIDCService, error) {
	t.Helper()
	userRepo := userfakes.NewFakeUserRepository()
	profileRepo := userfakes.NewFakeProfileRepository()
	rtRepo := userfakes.NewFakeRefreshTokenRepository()
	userRoleRepo := userfakes.NewFakeUserRoleRepository(userfakes.NewFakeRoleRepository())
	tokenGen := userfakes.NewFakeTokenGenerator()
	bus := sharedfakes.NewFakeEventBus()

	cfg := &config.Config{}
	cfg.OIDC.Enabled = true
	cfg.OIDC.Issuer = issuerURL
	cfg.OIDC.ClientID = "test-client"
	cfg.OIDC.ClientSecret = clientSecret
	cfg.OIDC.RedirectURL = "http://localhost/callback"

	ctx := gooidc.InsecureIssuerURLContext(context.Background(), issuerURL)
	return appuser.NewOIDCService(ctx, cfg, userRepo, profileRepo, rtRepo, userRoleRepo, tokenGen, bus)
}

func TestOIDCPing_Success(t *testing.T) {
	srv := newTestOIDCServer(t, "")
	defer srv.Close()
	// set issuer to test server URL after creation
	issuer := srv.URL
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		doc := map[string]any{
			"issuer":                 issuer,
			"authorization_endpoint": issuer + "/auth",
			"token_endpoint":         issuer + "/token",
			"jwks_uri":               issuer + "/jwks",
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(doc); err != nil {
			http.Error(w, "encode error", http.StatusInternalServerError)
		}
	})

	svc, err := newOIDCSvc(t, issuer, "supersecret")
	require.NoError(t, err)

	ctx := gooidc.InsecureIssuerURLContext(context.Background(), issuer)
	assert.NoError(t, svc.Ping(ctx))
}

func TestOIDCPing_Unreachable(t *testing.T) {
	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		doc := map[string]any{
			"issuer":                 srvURL,
			"authorization_endpoint": srvURL + "/auth",
			"token_endpoint":         srvURL + "/token",
			"jwks_uri":               srvURL + "/jwks",
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(doc); err != nil {
			http.Error(w, "encode error", http.StatusInternalServerError)
		}
	}))
	srvURL = srv.URL
	issuer := srv.URL
	svc, err := newOIDCSvc(t, issuer, "supersecret")
	require.NoError(t, err)

	srv.Close() // shut down so Ping cannot reach it

	ctx := gooidc.InsecureIssuerURLContext(context.Background(), issuer)
	pingErr := svc.Ping(ctx)
	require.Error(t, pingErr)
	assert.Contains(t, pingErr.Error(), "oidc ping")
}

func TestNewOIDCService_EmptyClientSecret(t *testing.T) {
	srv := newTestOIDCServer(t, "")
	issuer := srv.URL
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		doc := map[string]any{
			"issuer":                 issuer,
			"authorization_endpoint": issuer + "/auth",
			"token_endpoint":         issuer + "/token",
			"jwks_uri":               issuer + "/jwks",
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(doc); err != nil {
			http.Error(w, "encode error", http.StatusInternalServerError)
		}
	})
	defer srv.Close()

	_, err := newOIDCSvc(t, issuer, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "client secret")
}
