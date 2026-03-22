package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	appuser "github.com/gravity-zero/evey/internal/app/user"
	"github.com/gravity-zero/evey/internal/config"
	notifakes "github.com/gravity-zero/evey/internal/domain/notification/fakes"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	userfakes "github.com/gravity-zero/evey/internal/domain/user/fakes"
	"github.com/gravity-zero/evey/internal/pkg/auth"
)

// newMockOIDCServer returns an httptest.Server that serves minimal OIDC discovery + JWKS.
func newMockOIDCServer(t *testing.T) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		base := srv.URL
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                 base,
			"authorization_endpoint": base + "/auth",
			"token_endpoint":         base + "/token",
			"jwks_uri":               base + "/jwks",
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"keys":[]}`))
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func newTestOIDCSvc(t *testing.T) *appuser.OIDCService {
	t.Helper()
	oidcSrv := newMockOIDCServer(t)
	cfg := &config.Config{
		OIDC: config.OIDCConfig{
			Enabled:      true,
			Issuer:       oidcSrv.URL,
			ClientID:     "test-client",
			ClientSecret: "test-secret",
			RedirectURL:  "http://localhost/callback",
		},
	}
	svc, err := appuser.NewOIDCService(
		context.Background(), cfg,
		userfakes.NewFakeUserRepository(),
		userfakes.NewFakeProfileRepository(),
		userfakes.NewFakeRefreshTokenRepository(),
		userfakes.NewFakeUserRoleRepository(userfakes.NewFakeRoleRepository()),
		userfakes.NewFakeTokenGenerator(),
		nil,
	)
	if err != nil {
		t.Fatalf("NewOIDCService: %v", err)
	}
	return svc
}

func TestOIDCCallback_MissingState(t *testing.T) {
	svc := newTestOIDCSvc(t)
	mux := http.NewServeMux()
	registerOIDCRoutes(mux, svc, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/callback?code=abc", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestOIDCCallback_MissingCode(t *testing.T) {
	svc := newTestOIDCSvc(t)
	mux := http.NewServeMux()
	registerOIDCRoutes(mux, svc, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/callback?state=abc", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestOIDCCallback_InvalidState_CSRF(t *testing.T) {
	svc := newTestOIDCSvc(t)
	mux := http.NewServeMux()
	registerOIDCRoutes(mux, svc, false)

	// state with wrong signature
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/callback?code=abc&state=nonce.badsig", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid state, got %d", w.Code)
	}
}

func newTestJWTGen(t *testing.T) *auth.JWTGenerator {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	return auth.NewJWTGenerator(key, 15*time.Minute)
}

func TestPushRegister_NoAuth_401(t *testing.T) {
	tokenGen := newTestJWTGen(t)
	repo := notifakes.NewFakePushSubscriptionRepository()
	mux := http.NewServeMux()
	registerPushRoutes(mux, repo, userfakes.NewFakeProfileRepository(), userfakes.NewFakeUserRepository(), tokenGen, "test-vapid-key")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/push/register", strings.NewReader(`{"endpoint":"https://push.example.com/1","keys":{"p256dh":"abc","auth":"def"}}`))
	req.RemoteAddr = "10.1.1.1:9999"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestPushRegister_InvalidToken_401(t *testing.T) {
	tokenGen := newTestJWTGen(t)
	repo := notifakes.NewFakePushSubscriptionRepository()
	mux := http.NewServeMux()
	registerPushRoutes(mux, repo, userfakes.NewFakeProfileRepository(), userfakes.NewFakeUserRepository(), tokenGen, "test-vapid-key")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/push/register", strings.NewReader(`{"endpoint":"https://push.example.com/1","keys":{"p256dh":"abc","auth":"def"}}`))
	req.Header.Set("Authorization", "Bearer invalid.token.here")
	req.RemoteAddr = "10.1.1.2:9999"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestPushRegister_RateLimit_429(t *testing.T) {
	tokenGen := newTestJWTGen(t)
	repo := notifakes.NewFakePushSubscriptionRepository()
	mux := http.NewServeMux()
	registerPushRoutes(mux, repo, userfakes.NewFakeProfileRepository(), userfakes.NewFakeUserRepository(), tokenGen, "test-vapid-key")

	userID := fmt.Sprintf("rl-user-%d", time.Now().UnixNano())
	token, _ := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: userID, Username: "rluser"})

	var lastCode int
	for range 11 {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/push/register", strings.NewReader(`{"endpoint":"https://push.example.com/1","keys":{"p256dh":"abc","auth":"def"},"consent":true}`))
		req.Header.Set("Authorization", "Bearer "+token)
		req.RemoteAddr = "10.99.1.1:1234"
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		lastCode = w.Code
	}
	if lastCode != http.StatusTooManyRequests {
		t.Errorf("expected 429 after burst exceeded, got %d", lastCode)
	}
}

func TestPushRegister_ProfileOwnershipMismatch_403(t *testing.T) {
	tokenGen := newTestJWTGen(t)
	repo := notifakes.NewFakePushSubscriptionRepository()
	profileRepo := userfakes.NewFakeProfileRepository()
	_ = profileRepo.Create(context.Background(), &domainuser.Profile{ID: "p1", UserID: "u-owner", Name: "Owner"})
	mux := http.NewServeMux()
	registerPushRoutes(mux, repo, profileRepo, userfakes.NewFakeUserRepository(), tokenGen, "test-vapid-key")

	token, _ := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "u-attacker", Username: "attacker", ProfileID: "p1"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/push/register",
		strings.NewReader(`{"endpoint":"https://push.example.com/1","keys":{"p256dh":"abc","auth":"def"},"consent":true}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = "10.10.10.1:9999"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for profile ownership mismatch, got %d", w.Code)
	}
}

func TestPushRegister_ProfileOwnershipValid_200(t *testing.T) {
	tokenGen := newTestJWTGen(t)
	repo := notifakes.NewFakePushSubscriptionRepository()
	profileRepo := userfakes.NewFakeProfileRepository()
	_ = profileRepo.Create(context.Background(), &domainuser.Profile{ID: "p-mine", UserID: "u-legit", Name: "Mine"})
	userRepo := userfakes.NewFakeUserRepository()
	_ = userRepo.Create(context.Background(), &domainuser.User{ID: "u-legit", Username: "legit", Enabled: true})
	mux := http.NewServeMux()
	registerPushRoutes(mux, repo, profileRepo, userRepo, tokenGen, "test-vapid-key")

	token, _ := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "u-legit", Username: "legit", ProfileID: "p-mine"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/push/register",
		strings.NewReader(`{"endpoint":"https://push.example.com/1","keys":{"p256dh":"abc","auth":"def"},"consent":true}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = "10.11.11.1:9999"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for valid profile ownership, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestOIDCLogin_RedirectsToProvider(t *testing.T) {
	svc := newTestOIDCSvc(t)
	mux := http.NewServeMux()
	registerOIDCRoutes(mux, svc, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/login", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("expected 302 redirect, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc == "" {
		t.Error("expected Location header")
	}
	if !strings.Contains(loc, "/auth") {
		t.Errorf("expected redirect to provider /auth endpoint, got %q", loc)
	}
}

func TestOIDCCallback_ValidState_TokenExchangeFails_RedirectsToError(t *testing.T) {
	svc := newTestOIDCSvc(t)
	mux := http.NewServeMux()
	registerOIDCRoutes(mux, svc, false)

	// Generate valid signed state via AuthURL.
	authURL := svc.AuthURL("testnonce")
	// Extract state param from the redirect URL.
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse authURL: %v", err)
	}
	validState := parsed.Query().Get("state")
	if validState == "" {
		t.Fatal("state param missing from authURL")
	}

	// Send callback with valid state but a bogus code — provider token exchange will fail,
	// so the handler redirects to /login?oidc_error=1.
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/auth/oidc/callback?code=bogus-code&state="+validState, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Valid state passes CSRF check; code exchange fails → redirect to error page.
	if w.Code != http.StatusFound {
		t.Errorf("expected 302 redirect on token exchange failure, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "oidc_error") {
		t.Errorf("expected oidc_error in redirect, got %q", loc)
	}
}

func TestOIDCSession_NoCookie_Unauthorized(t *testing.T) {
	svc := newTestOIDCSvc(t)
	mux := http.NewServeMux()
	registerOIDCRoutes(mux, svc, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/session", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without session cookie, got %d", w.Code)
	}
}

func TestOIDCSession_ValidCookie_ReturnsToken(t *testing.T) {
	svc := newTestOIDCSvc(t)
	mux := http.NewServeMux()
	registerOIDCRoutes(mux, svc, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/session", nil)
	req.AddCookie(&http.Cookie{Name: oidcSessionCookieName, Value: "my-access-token"})
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with session cookie, got %d", w.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["accessToken"] != "my-access-token" {
		t.Errorf("expected accessToken=my-access-token, got %v", resp["accessToken"])
	}
	// Cookie should be cleared (MaxAge -1).
	var cleared bool
	for _, c := range w.Result().Cookies() {
		if c.Name == oidcSessionCookieName && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("session cookie should be cleared after consumption")
	}
}

func TestOIDCConfig_ReturnsEnabled(t *testing.T) {
	svc := newTestOIDCSvc(t)
	mux := http.NewServeMux()
	registerOIDCRoutes(mux, svc, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/config", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["enabled"] != true {
		t.Errorf("expected enabled=true, got %v", resp["enabled"])
	}
}

func TestExtractBearerToken_Header(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer my-token")
	got := extractBearerToken(req)
	if got != "my-token" {
		t.Errorf("expected 'my-token', got %q", got)
	}
}

func TestExtractBearerToken_QueryParam(t *testing.T) {
	req := httptest.NewRequest("GET", "/?token=query-token", nil)
	got := extractBearerToken(req)
	if got != "query-token" {
		t.Errorf("expected 'query-token', got %q", got)
	}
}

func TestExtractBearerToken_Empty(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	got := extractBearerToken(req)
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestPushRegister_NoConsent_400(t *testing.T) {
	tokenGen := newTestJWTGen(t)
	repo := notifakes.NewFakePushSubscriptionRepository()
	mux := http.NewServeMux()
	registerPushRoutes(mux, repo, userfakes.NewFakeProfileRepository(), userfakes.NewFakeUserRepository(), tokenGen, "test-vapid-key")

	token, _ := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "u1", Username: "user"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/push/register",
		strings.NewReader(`{"endpoint":"https://push.example.com/1","keys":{"p256dh":"abc","auth":"def"},"consent":false}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = "10.2.2.1:9999"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing consent, got %d", w.Code)
	}
}

func TestPushRegister_Success_200(t *testing.T) {
	tokenGen := newTestJWTGen(t)
	repo := notifakes.NewFakePushSubscriptionRepository()
	userRepo := userfakes.NewFakeUserRepository()
	_ = userRepo.Create(context.Background(), &domainuser.User{ID: "u1", Username: "user", Enabled: true})
	mux := http.NewServeMux()
	registerPushRoutes(mux, repo, userfakes.NewFakeProfileRepository(), userRepo, tokenGen, "test-vapid-key")

	token, _ := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "u1", Username: "user"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/push/register",
		strings.NewReader(`{"endpoint":"https://push.example.com/1","keys":{"p256dh":"abc","auth":"def"},"consent":true}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = "10.3.3.1:9999"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestPushList_NoAuth_401(t *testing.T) {
	tokenGen := newTestJWTGen(t)
	repo := notifakes.NewFakePushSubscriptionRepository()
	mux := http.NewServeMux()
	registerPushRoutes(mux, repo, userfakes.NewFakeProfileRepository(), userfakes.NewFakeUserRepository(), tokenGen, "test-vapid-key")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/push/subscriptions", nil)
	req.RemoteAddr = "10.4.4.1:9999"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestPushDelete_NoAuth_401(t *testing.T) {
	tokenGen := newTestJWTGen(t)
	repo := notifakes.NewFakePushSubscriptionRepository()
	mux := http.NewServeMux()
	registerPushRoutes(mux, repo, userfakes.NewFakeProfileRepository(), userfakes.NewFakeUserRepository(), tokenGen, "test-vapid-key")

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/push/subscriptions", strings.NewReader(`{"endpoint":"https://push.example.com/1"}`))
	req.RemoteAddr = "10.5.5.1:9999"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestRequireAdmin_NoToken(t *testing.T) {
	tv := invalidValidator()
	handler := requireAdmin(tv, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestRequireAdmin_NonAdmin(t *testing.T) {
	tv := validUserValidator(false)
	handler := requireAdmin(tv, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestRequireAdmin_Admin(t *testing.T) {
	tv := validUserValidator(true)
	handler := requireAdmin(tv, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
