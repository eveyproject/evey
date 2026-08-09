package user

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"golang.org/x/oauth2"

	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
)

// --- test OIDC infra ---

type testOIDCServer struct {
	srv    *httptest.Server
	key    *rsa.PrivateKey
	keyID  string
	issuer string
}

func newTestOIDCServer(t *testing.T) *testOIDCServer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	s := &testOIDCServer{key: key, keyID: "test-key-1"}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", s.handleDiscovery)
	mux.HandleFunc("/token", s.handleToken)
	mux.HandleFunc("/jwks", s.handleJWKS)

	s.srv = httptest.NewServer(mux)
	s.issuer = s.srv.URL
	return s
}

func (s *testOIDCServer) close() { s.srv.Close() }

func (s *testOIDCServer) handleDiscovery(w http.ResponseWriter, _ *http.Request) {
	doc := map[string]any{
		"issuer":                 s.issuer,
		"authorization_endpoint": s.issuer + "/auth",
		"token_endpoint":         s.issuer + "/token",
		"jwks_uri":               s.issuer + "/jwks",
		"end_session_endpoint":   s.issuer + "/logout",
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(doc)
}

func (s *testOIDCServer) handleJWKS(w http.ResponseWriter, _ *http.Request) {
	jwk := jose.JSONWebKey{Key: &s.key.PublicKey, KeyID: s.keyID, Algorithm: "RS256", Use: "sig"}
	set := jose.JSONWebKeySet{Keys: []jose.JSONWebKey{jwk}}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(set)
}

func (s *testOIDCServer) signIDToken(t *testing.T, claims map[string]any, clientID string) string {
	t.Helper()
	signerOpts := jose.SignerOptions{}
	signerOpts.WithHeader(jose.HeaderKey("kid"), s.keyID)
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: s.key}, &signerOpts)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	stdClaims := jwt.Claims{
		Issuer:   s.issuer,
		Subject:  claims["sub"].(string),
		Audience: jwt.Audience{clientID},
		IssuedAt: jwt.NewNumericDate(now),
		Expiry:   jwt.NewNumericDate(now.Add(5 * time.Minute)),
	}

	raw, err := jwt.Signed(signer).Claims(stdClaims).Claims(claims).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func (s *testOIDCServer) handleToken(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"access_token": "test-access-token",
		"token_type":   "Bearer",
		"expires_in":   3600,
	})
}

func (s *testOIDCServer) handleTokenWithIDToken(idToken string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "test-access-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
			"id_token":     idToken,
		})
	}
}

// buildOIDCService constructs an OIDCService with a test OIDC server, bypassing NewOIDCService discovery
func buildOIDCService(t *testing.T, oidcSrv *testOIDCServer, opts ...func(*OIDCService)) *OIDCService {
	t.Helper()
	clientID := "test-client"

	ctx := gooidc.ClientContext(context.Background(), oidcSrv.srv.Client())
	provider, err := gooidc.NewProvider(ctx, oidcSrv.issuer)
	if err != nil {
		t.Fatal(err)
	}
	verifier := provider.Verifier(&gooidc.Config{ClientID: clientID})

	svc := &OIDCService{
		provider:    provider,
		verifier:    verifier,
		userRepo:    userfakes.NewFakeUserRepository(),
		profiles:    userfakes.NewFakeProfileRepository(),
		refreshTkns: userfakes.NewFakeRefreshTokenRepository(),
		userRoles:   userfakes.NewFakeUserRoleRepository(userfakes.NewFakeRoleRepository()),
		tokens:      userfakes.NewFakeTokenGenerator(),
		events:      sharedfakes.NewFakeEventBus(),
		stateSecret: []byte("test-secret-32-bytes-long-enough"),
		issuer:      oidcSrv.issuer,
		oauth2Cfg: oauth2.Config{
			ClientID:     clientID,
			ClientSecret: "test-secret",
			RedirectURL:  "http://localhost/callback",
			Endpoint:     provider.Endpoint(),
			Scopes:       []string{gooidc.ScopeOpenID, "profile", "email"},
		},
	}
	for _, o := range opts {
		o(svc)
	}
	return svc
}

// --- IsEmailAllowed ---

func TestIsEmailAllowed(t *testing.T) {
	tests := []struct {
		name    string
		email   string
		domains []string
		want    bool
	}{
		{"empty allowlist allows all", "user@any.com", nil, true},
		{"allowed domain", "user@example.com", []string{"example.com"}, true},
		{"disallowed domain", "user@evil.com", []string{"example.com"}, false},
		{"case insensitive", "user@Example.COM", []string{"example.com"}, true},
		{"no @ in email", "invalid-email", []string{"example.com"}, false},
		{"multiple domains second match", "user@corp.io", []string{"example.com", "corp.io"}, true},
		{"empty email", "", []string{"example.com"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsEmailAllowed(tt.email, tt.domains)
			if got != tt.want {
				t.Errorf("IsEmailAllowed(%q, %v) = %v, want %v", tt.email, tt.domains, got, tt.want)
			}
		})
	}
}

// --- AuthURL + ValidateState round-trip ---

func TestAuthURL_ContainsSignedState(t *testing.T) {
	oidcSrv := newTestOIDCServer(t)
	defer oidcSrv.close()
	svc := buildOIDCService(t, oidcSrv)

	url := svc.AuthURL("my-nonce")
	if url == "" {
		t.Fatal("AuthURL returned empty string")
	}
	if !strings.Contains(url, "my-nonce") {
		t.Errorf("URL should contain nonce, got %s", url)
	}
	// state format: ts|nonce.sig - must contain pipe separator
	if !strings.Contains(url, "%7C") && !strings.Contains(url, "|") {
		t.Errorf("URL should contain timestamp pipe separator, got %s", url)
	}
}

func TestValidateState_ValidRoundTrip(t *testing.T) {
	oidcSrv := newTestOIDCServer(t)
	defer oidcSrv.close()
	svc := buildOIDCService(t, oidcSrv)

	ts := strconv.FormatInt(time.Now().Unix(), 10)
	payload := ts + "|nonce123"
	signed := svc.signState(payload)
	nonce, err := svc.ValidateState(payload + "." + signed)
	if err != nil {
		t.Fatalf("ValidateState should succeed: %v", err)
	}
	if nonce != "nonce123" {
		t.Errorf("got nonce %q, want %q", nonce, "nonce123")
	}
}

func TestValidateState_InvalidFormat(t *testing.T) {
	oidcSrv := newTestOIDCServer(t)
	defer oidcSrv.close()
	svc := buildOIDCService(t, oidcSrv)

	_, err := svc.ValidateState("no-dot-separator")
	if err == nil {
		t.Fatal("expected error for state without dot")
	}
	if err.Error() != "invalid state" {
		t.Errorf("got error %q, want %q", err.Error(), "invalid state")
	}
}

func TestValidateState_WrongSignature(t *testing.T) {
	oidcSrv := newTestOIDCServer(t)
	defer oidcSrv.close()
	svc := buildOIDCService(t, oidcSrv)

	ts := strconv.FormatInt(time.Now().Unix(), 10)
	_, err := svc.ValidateState(ts + "|nonce.wrong-signature")
	if err == nil {
		t.Fatal("expected error for wrong signature")
	}
	if err.Error() != "state signature mismatch" {
		t.Errorf("got error %q, want %q", err.Error(), "state signature mismatch")
	}
}

func TestValidateState_DifferentSecretsFail(t *testing.T) {
	oidcSrv := newTestOIDCServer(t)
	defer oidcSrv.close()
	svc1 := buildOIDCService(t, oidcSrv)
	svc2 := buildOIDCService(t, oidcSrv, func(s *OIDCService) {
		s.stateSecret = []byte("different-secret-key-very-long!!")
	})

	ts := strconv.FormatInt(time.Now().Unix(), 10)
	payload := ts + "|nonce"
	sig := svc1.signState(payload)
	_, err := svc2.ValidateState(payload + "." + sig)
	if err == nil {
		t.Fatal("expected mismatch with different secrets")
	}
}

func TestValidateState_Expired(t *testing.T) {
	oidcSrv := newTestOIDCServer(t)
	defer oidcSrv.close()
	svc := buildOIDCService(t, oidcSrv)

	ts := strconv.FormatInt(time.Now().Add(-11*time.Minute).Unix(), 10)
	payload := ts + "|nonce-old"
	sig := svc.signState(payload)
	_, err := svc.ValidateState(payload + "." + sig)
	if !errors.Is(err, ErrStateExpired) {
		t.Errorf("expected ErrStateExpired, got %v", err)
	}
}

func TestValidateState_NotExpired(t *testing.T) {
	oidcSrv := newTestOIDCServer(t)
	defer oidcSrv.close()
	svc := buildOIDCService(t, oidcSrv)

	ts := strconv.FormatInt(time.Now().Add(-9*time.Minute).Unix(), 10)
	payload := ts + "|nonce-fresh"
	sig := svc.signState(payload)
	nonce, err := svc.ValidateState(payload + "." + sig)
	if err != nil {
		t.Fatalf("expected valid state, got %v", err)
	}
	if nonce != "nonce-fresh" {
		t.Errorf("got nonce %q, want %q", nonce, "nonce-fresh")
	}
}

func TestValidateState_MissingTimestamp(t *testing.T) {
	oidcSrv := newTestOIDCServer(t)
	defer oidcSrv.close()
	svc := buildOIDCService(t, oidcSrv)

	// Old format without timestamp - should be rejected
	sig := svc.signState("nonce-only")
	_, err := svc.ValidateState("nonce-only." + sig)
	if err == nil {
		t.Fatal("expected error for state without timestamp")
	}
}

// --- EndSessionEndpoint ---

func TestEndSessionEndpoint_ReturnsURL(t *testing.T) {
	oidcSrv := newTestOIDCServer(t)
	defer oidcSrv.close()
	svc := buildOIDCService(t, oidcSrv)

	ep := svc.EndSessionEndpoint()
	want := oidcSrv.issuer + "/logout"
	if ep != want {
		t.Errorf("EndSessionEndpoint() = %q, want %q", ep, want)
	}
}

// --- HandleCallback ---

func TestHandleCallback_MissingIDToken(t *testing.T) {
	oidcSrv := newTestOIDCServer(t)
	defer oidcSrv.close()
	svc := buildOIDCService(t, oidcSrv)

	_, _, _, err := svc.HandleCallback(context.Background(), "auth-code", "")
	if err == nil {
		t.Fatal("expected error when id_token missing")
	}
	if err.Error() != "missing id_token in response" {
		t.Errorf("got error %q, want 'missing id_token in response'", err.Error())
	}
}

func TestHandleCallback_NewUser(t *testing.T) {
	oidcSrv := newTestOIDCServer(t)
	defer oidcSrv.close()

	clientID := "test-client"
	idToken := oidcSrv.signIDToken(t, map[string]any{
		"sub":                "oidc-sub-123",
		"email":              "alice@example.com",
		"preferred_username": "alice",
		"name":               "Alice",
	}, clientID)

	// Replace the token handler to return the id_token
	oidcSrv.srv.Config.Handler.(*http.ServeMux).HandleFunc("/token2", func(w http.ResponseWriter, r *http.Request) {})
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			oidcSrv.handleTokenWithIDToken(idToken)(w, r)
			return
		}
		oidcSrv.srv.Config.Handler.ServeHTTP(w, r)
	}))
	defer tokenSrv.Close()

	// Need a fresh server with consistent URLs for token endpoint
	oidcSrv2 := newTestOIDCServer(t)
	defer oidcSrv2.close()

	// Override the token endpoint handler
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", oidcSrv2.handleDiscovery)
	mux.HandleFunc("/jwks", oidcSrv2.handleJWKS)
	mux.Handle("/token", oidcSrv2.handleTokenWithIDToken(
		oidcSrv2.signIDToken(t, map[string]any{
			"sub":                "oidc-sub-123",
			"email":              "alice@example.com",
			"preferred_username": "alice",
			"name":               "Alice",
		}, "test-client"),
	))
	oidcSrv2.srv.Config.Handler = mux

	svc := buildOIDCService(t, oidcSrv2)
	userRepo := svc.userRepo.(*userfakes.FakeUserRepository)
	events := svc.events.(*sharedfakes.FakeEventBus)

	accessTk, refreshTk, rawIDTk, err := svc.HandleCallback(context.Background(), "auth-code", "")
	if err != nil {
		t.Fatalf("HandleCallback failed: %v", err)
	}
	if accessTk == "" {
		t.Error("expected non-empty access token")
	}
	if refreshTk == "" {
		t.Error("expected non-empty refresh token")
	}
	if rawIDTk == "" {
		t.Error("expected non-empty raw OIDC id_token")
	}

	// Verify user was created
	users, _ := userRepo.List(context.Background())
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
	u := users[0]
	if u.Username != "alice" {
		t.Errorf("username = %q, want 'alice'", u.Username)
	}
	if u.Email != "alice@example.com" {
		t.Errorf("email = %q, want 'alice@example.com'", u.Email)
	}
	if u.OIDCSubject == nil || *u.OIDCSubject != "oidc-sub-123" {
		t.Error("OIDC subject not set correctly")
	}
	if !u.Enabled {
		t.Error("new user should be enabled")
	}
	if u.IsAdmin {
		t.Error("new OIDC user should not be admin")
	}

	// Verify events published
	evts := events.PublishedEvents()
	foundUserCreated := false
	foundLogin := false
	for _, e := range evts {
		switch e.EventName() {
		case "user.created":
			foundUserCreated = true
		case "user.login.succeeded":
			foundLogin = true
		}
	}
	if !foundUserCreated {
		t.Error("expected user.created event")
	}
	if !foundLogin {
		t.Error("expected user.login.succeeded event")
	}
}

func TestHandleCallback_ExistingUser(t *testing.T) {
	oidcSrv := newTestOIDCServer(t)
	defer oidcSrv.close()

	sub := "existing-sub"
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", oidcSrv.handleDiscovery)
	mux.HandleFunc("/jwks", oidcSrv.handleJWKS)
	mux.Handle("/token", oidcSrv.handleTokenWithIDToken(
		oidcSrv.signIDToken(t, map[string]any{
			"sub":                sub,
			"email":              "bob@example.com",
			"preferred_username": "bob",
		}, "test-client"),
	))
	oidcSrv.srv.Config.Handler = mux

	svc := buildOIDCService(t, oidcSrv)
	userRepo := svc.userRepo.(*userfakes.FakeUserRepository)
	issuer := oidcSrv.issuer

	existingUser := &domainuser.User{
		ID:          "usr-existing",
		Username:    "bob",
		Email:       "bob@example.com",
		Enabled:     true,
		OIDCSubject: &sub,
		OIDCIssuer:  &issuer,
		CreatedAt:   time.Now().Add(-24 * time.Hour),
		UpdatedAt:   time.Now().Add(-24 * time.Hour),
	}
	userRepo.Create(context.Background(), existingUser)

	// Also create a default profile
	profileRepo := svc.profiles.(*userfakes.FakeProfileRepository)
	profileRepo.Create(context.Background(), &domainuser.Profile{
		ID:        "prf-default",
		UserID:    "usr-existing",
		Name:      "bob",
		IsDefault: true,
	})

	accessTk, refreshTk, _, err := svc.HandleCallback(context.Background(), "auth-code", "")
	if err != nil {
		t.Fatalf("HandleCallback failed: %v", err)
	}
	if accessTk == "" || refreshTk == "" {
		t.Error("expected non-empty tokens")
	}

	// Should not create a new user
	users, _ := userRepo.List(context.Background())
	if len(users) != 1 {
		t.Fatalf("expected 1 user (no new creation), got %d", len(users))
	}

	// Verify login event only, no user.created
	events := svc.events.(*sharedfakes.FakeEventBus)
	evts := events.PublishedEvents()
	for _, e := range evts {
		if e.EventName() == "user.created" {
			t.Error("should not publish user.created for existing user")
		}
	}
}

func TestHandleCallback_DisabledUser(t *testing.T) {
	oidcSrv := newTestOIDCServer(t)
	defer oidcSrv.close()

	sub := "disabled-sub"
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", oidcSrv.handleDiscovery)
	mux.HandleFunc("/jwks", oidcSrv.handleJWKS)
	mux.Handle("/token", oidcSrv.handleTokenWithIDToken(
		oidcSrv.signIDToken(t, map[string]any{
			"sub":   sub,
			"email": "disabled@example.com",
		}, "test-client"),
	))
	oidcSrv.srv.Config.Handler = mux

	svc := buildOIDCService(t, oidcSrv)
	userRepo := svc.userRepo.(*userfakes.FakeUserRepository)
	issuer := oidcSrv.issuer

	userRepo.Create(context.Background(), &domainuser.User{
		ID:          "usr-disabled",
		Username:    "disabled",
		Enabled:     false,
		OIDCSubject: &sub,
		OIDCIssuer:  &issuer,
	})

	_, _, _, err := svc.HandleCallback(context.Background(), "auth-code", "")
	if !errors.Is(err, ErrUserDisabled) {
		t.Errorf("expected ErrUserDisabled, got %v", err)
	}
}

func TestHandleCallback_EmailDomainNotAllowed(t *testing.T) {
	oidcSrv := newTestOIDCServer(t)
	defer oidcSrv.close()

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", oidcSrv.handleDiscovery)
	mux.HandleFunc("/jwks", oidcSrv.handleJWKS)
	mux.Handle("/token", oidcSrv.handleTokenWithIDToken(
		oidcSrv.signIDToken(t, map[string]any{
			"sub":                "new-sub",
			"email":              "user@evil.com",
			"preferred_username": "hacker",
		}, "test-client"),
	))
	oidcSrv.srv.Config.Handler = mux

	svc := buildOIDCService(t, oidcSrv, func(s *OIDCService) {
		s.allowedEmailDomains = []string{"example.com"}
	})

	_, _, _, err := svc.HandleCallback(context.Background(), "auth-code", "")
	if !errors.Is(err, ErrEmailDomainNotAllowed) {
		t.Errorf("expected ErrEmailDomainNotAllowed, got %v", err)
	}
}

func TestHandleCallback_AccessTokenGenerationError(t *testing.T) {
	oidcSrv := newTestOIDCServer(t)
	defer oidcSrv.close()

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", oidcSrv.handleDiscovery)
	mux.HandleFunc("/jwks", oidcSrv.handleJWKS)
	mux.Handle("/token", oidcSrv.handleTokenWithIDToken(
		oidcSrv.signIDToken(t, map[string]any{
			"sub":                "sub-tknerr",
			"email":              "user@example.com",
			"preferred_username": "user",
		}, "test-client"),
	))
	oidcSrv.srv.Config.Handler = mux

	tokenGen := userfakes.NewFakeTokenGenerator()
	svc := buildOIDCService(t, oidcSrv, func(s *OIDCService) {
		s.tokens = tokenGen
	})

	// Make GenerateAccessToken fail via Err field - but we need it to succeed for refresh.
	// Instead, use a custom token generator
	tokenGen.Err = errors.New("token gen boom")

	_, _, _, err := svc.HandleCallback(context.Background(), "auth-code", "")
	if err == nil {
		t.Fatal("expected error when token generation fails")
	}
	if !strings.Contains(err.Error(), "generate access token") {
		t.Errorf("error should mention 'generate access token', got %q", err.Error())
	}
}

func TestHandleCallback_UserRepoCreateError(t *testing.T) {
	oidcSrv := newTestOIDCServer(t)
	defer oidcSrv.close()

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", oidcSrv.handleDiscovery)
	mux.HandleFunc("/jwks", oidcSrv.handleJWKS)
	mux.Handle("/token", oidcSrv.handleTokenWithIDToken(
		oidcSrv.signIDToken(t, map[string]any{
			"sub":                "sub-repoerr",
			"email":              "user@example.com",
			"preferred_username": "user",
		}, "test-client"),
	))
	oidcSrv.srv.Config.Handler = mux

	svc := buildOIDCService(t, oidcSrv, func(s *OIDCService) {
		repo := userfakes.NewFakeUserRepository()
		repo.Err = errors.New("db connection lost")
		s.userRepo = repo
	})

	_, _, _, err := svc.HandleCallback(context.Background(), "auth-code", "")
	if err == nil {
		t.Fatal("expected error when user repo fails")
	}
}

func TestHandleCallback_UsernameFromEmail(t *testing.T) {
	oidcSrv := newTestOIDCServer(t)
	defer oidcSrv.close()

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", oidcSrv.handleDiscovery)
	mux.HandleFunc("/jwks", oidcSrv.handleJWKS)
	mux.Handle("/token", oidcSrv.handleTokenWithIDToken(
		oidcSrv.signIDToken(t, map[string]any{
			"sub":   "sub-nousername",
			"email": "jane@example.com",
		}, "test-client"),
	))
	oidcSrv.srv.Config.Handler = mux

	svc := buildOIDCService(t, oidcSrv)
	userRepo := svc.userRepo.(*userfakes.FakeUserRepository)

	_, _, _, err := svc.HandleCallback(context.Background(), "auth-code", "")
	if err != nil {
		t.Fatalf("HandleCallback failed: %v", err)
	}

	users, _ := userRepo.List(context.Background())
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
	if users[0].Username != "jane" {
		t.Errorf("username = %q, want 'jane' (derived from email)", users[0].Username)
	}
}

func TestHandleCallback_UsernameFallbackHex(t *testing.T) {
	oidcSrv := newTestOIDCServer(t)
	defer oidcSrv.close()

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", oidcSrv.handleDiscovery)
	mux.HandleFunc("/jwks", oidcSrv.handleJWKS)
	mux.Handle("/token", oidcSrv.handleTokenWithIDToken(
		oidcSrv.signIDToken(t, map[string]any{
			"sub": "sub-noemail",
		}, "test-client"),
	))
	oidcSrv.srv.Config.Handler = mux

	svc := buildOIDCService(t, oidcSrv)
	userRepo := svc.userRepo.(*userfakes.FakeUserRepository)

	_, _, _, err := svc.HandleCallback(context.Background(), "auth-code", "")
	if err != nil {
		t.Fatalf("HandleCallback failed: %v", err)
	}

	users, _ := userRepo.List(context.Background())
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
	if !strings.Contains(users[0].Username, "oidc-") {
		t.Errorf("username = %q, want prefix 'oidc-' for hex fallback", users[0].Username)
	}
}

func TestHandleCallback_ProfileWithRoles(t *testing.T) {
	oidcSrv := newTestOIDCServer(t)
	defer oidcSrv.close()

	sub := "sub-roles"
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", oidcSrv.handleDiscovery)
	mux.HandleFunc("/jwks", oidcSrv.handleJWKS)
	mux.Handle("/token", oidcSrv.handleTokenWithIDToken(
		oidcSrv.signIDToken(t, map[string]any{
			"sub":                sub,
			"email":              "admin@example.com",
			"preferred_username": "admin",
		}, "test-client"),
	))
	oidcSrv.srv.Config.Handler = mux

	roleRepo := userfakes.NewFakeRoleRepository()
	roleRepo.Create(context.Background(), &domainuser.Role{
		ID:          "role-1",
		Name:        "admin",
		Permissions: []string{"media.write", "user.manage"},
	})
	userRoleRepo := userfakes.NewFakeUserRoleRepository(roleRepo)

	svc := buildOIDCService(t, oidcSrv, func(s *OIDCService) {
		s.userRoles = userRoleRepo
	})

	userRepo := svc.userRepo.(*userfakes.FakeUserRepository)
	issuer := oidcSrv.issuer
	userRepo.Create(context.Background(), &domainuser.User{
		ID:          "usr-roles",
		Username:    "admin",
		Enabled:     true,
		OIDCSubject: &sub,
		OIDCIssuer:  &issuer,
	})
	userRoleRepo.AssignRole(context.Background(), "usr-roles", "role-1")

	// Create non-default profile to test fallback to first profile
	profileRepo := svc.profiles.(*userfakes.FakeProfileRepository)
	profileRepo.Create(context.Background(), &domainuser.Profile{
		ID:        "prf-1",
		UserID:    "usr-roles",
		Name:      "admin",
		IsDefault: false,
	})

	accessTk, _, _, err := svc.HandleCallback(context.Background(), "auth-code", "")
	if err != nil {
		t.Fatalf("HandleCallback failed: %v", err)
	}
	if accessTk == "" {
		t.Error("expected non-empty access token")
	}
}

func TestHandleCallback_RefreshTokenError(t *testing.T) {
	oidcSrv := newTestOIDCServer(t)
	defer oidcSrv.close()

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", oidcSrv.handleDiscovery)
	mux.HandleFunc("/jwks", oidcSrv.handleJWKS)
	mux.Handle("/token", oidcSrv.handleTokenWithIDToken(
		oidcSrv.signIDToken(t, map[string]any{
			"sub":                "sub-refresherr",
			"email":              "user@example.com",
			"preferred_username": "user",
		}, "test-client"),
	))
	oidcSrv.srv.Config.Handler = mux

	tokenGen := userfakes.NewFakeTokenGenerator()
	svc := buildOIDCService(t, oidcSrv, func(s *OIDCService) {
		s.tokens = tokenGen
	})

	// Use a wrapper that fails only on refresh token generation
	svc.tokens = &failOnRefreshTokenGen{
		delegate:        tokenGen,
		refreshTokenErr: errors.New("refresh gen boom"),
	}

	_, _, _, err := svc.HandleCallback(context.Background(), "auth-code", "")
	if err == nil {
		t.Fatal("expected error when refresh token generation fails")
	}
	if !strings.Contains(err.Error(), "generate refresh token") {
		t.Errorf("error should mention refresh token, got %q", err.Error())
	}
}

type failOnRefreshTokenGen struct {
	delegate        domainuser.TokenGenerator
	refreshTokenErr error
}

func (f *failOnRefreshTokenGen) GenerateAccessToken(c domainuser.Claims) (string, error) {
	return f.delegate.GenerateAccessToken(c)
}
func (f *failOnRefreshTokenGen) GenerateRefreshToken() (string, error) {
	return "", f.refreshTokenErr
}
func (f *failOnRefreshTokenGen) ValidateAccessToken(token string) (*domainuser.Claims, error) {
	return f.delegate.ValidateAccessToken(token)
}
func (f *failOnRefreshTokenGen) GeneratePINToken(p string, ttl time.Duration) (string, error) {
	return f.delegate.GeneratePINToken(p, ttl)
}
func (f *failOnRefreshTokenGen) ParsePINToken(raw string) (string, error) {
	return f.delegate.ParsePINToken(raw)
}

func TestHandleCallback_TokenExchangeError(t *testing.T) {
	oidcSrv := newTestOIDCServer(t)
	defer oidcSrv.close()

	// Override token endpoint to return error
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", oidcSrv.handleDiscovery)
	mux.HandleFunc("/jwks", oidcSrv.handleJWKS)
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
	})
	oidcSrv.srv.Config.Handler = mux

	svc := buildOIDCService(t, oidcSrv)

	_, _, _, err := svc.HandleCallback(context.Background(), "bad-code", "")
	if err == nil {
		t.Fatal("expected error on token exchange failure")
	}
	if !strings.Contains(err.Error(), "token exchange") {
		t.Errorf("error should mention 'token exchange', got %q", err.Error())
	}
}

// Ping tests removed - OIDCService.Ping was deleted (dead code cleanup)

// generateAndStoreRefreshToken tests kept in command_service_test.go (duplicate removed)

// --- gremlins gap-closing tests for error branches ---

// captureSlogDuring redirects slog default output to a buffer for the duration of fn.
func captureSlogDuring(t *testing.T, fn func()) string {
	t.Helper()
	var buf strings.Builder
	oldDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(oldDefault)
	fn()
	return buf.String()
}

// wrappers for per-method error injection

type errUserRepo struct {
	domainuser.UserRepository
	updateErr error
}

func (r *errUserRepo) Update(ctx context.Context, u *domainuser.User) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	return r.UserRepository.Update(ctx, u)
}

type errProfileRepo struct {
	domainuser.ProfileRepository
	createErr    error
	getByUserErr error
	getByIDErr   error
}

func (r *errProfileRepo) Create(ctx context.Context, p *domainuser.Profile) error {
	if r.createErr != nil {
		return r.createErr
	}
	return r.ProfileRepository.Create(ctx, p)
}

func (r *errProfileRepo) GetByUserID(ctx context.Context, userID shared.UserID) ([]*domainuser.Profile, error) {
	if r.getByUserErr != nil {
		return nil, r.getByUserErr
	}
	return r.ProfileRepository.GetByUserID(ctx, userID)
}

func (r *errProfileRepo) GetByID(ctx context.Context, id shared.ProfileID) (*domainuser.Profile, error) {
	if r.getByIDErr != nil {
		return nil, r.getByIDErr
	}
	return r.ProfileRepository.GetByID(ctx, id)
}

type errUserRoleRepo struct {
	domainuser.UserRoleRepository
	getErr error
}

func (r *errUserRoleRepo) GetRolesByUserID(ctx context.Context, userID shared.UserID) ([]*domainuser.Role, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	return r.UserRoleRepository.GetRolesByUserID(ctx, userID)
}

type errRefreshRepo struct {
	domainuser.RefreshTokenRepository
	deleteByUserErr error
}

func (r *errRefreshRepo) DeleteByUserID(ctx context.Context, userID shared.UserID) error {
	if r.deleteByUserErr != nil {
		return r.deleteByUserErr
	}
	return r.RefreshTokenRepository.DeleteByUserID(ctx, userID)
}

// helper: build a fresh OIDC server and service, with token endpoint serving a signed id_token for the given claims.
func setupCallbackSvc(t *testing.T, claims map[string]any, opts ...func(*OIDCService)) (*OIDCService, *testOIDCServer) {
	t.Helper()
	oidcSrv := newTestOIDCServer(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", oidcSrv.handleDiscovery)
	mux.HandleFunc("/jwks", oidcSrv.handleJWKS)
	mux.Handle("/token", oidcSrv.handleTokenWithIDToken(oidcSrv.signIDToken(t, claims, "test-client")))
	oidcSrv.srv.Config.Handler = mux
	svc := buildOIDCService(t, oidcSrv, opts...)
	return svc, oidcSrv
}

// L193-194: GetRolesByUserID errors -> slog.Warn, callback continues
func TestHandleCallback_RolesRepoError_ContinuesWithoutRoles(t *testing.T) {
	sub := "sub-rolerr"
	svc, oidcSrv := setupCallbackSvc(t, map[string]any{
		"sub":                sub,
		"email":              "u@example.com",
		"preferred_username": "u",
	})
	defer oidcSrv.close()

	userRepo := svc.userRepo.(*userfakes.FakeUserRepository)
	issuer := oidcSrv.issuer
	userRepo.Create(context.Background(), &domainuser.User{
		ID: "usr-rolerr", Username: "u", Enabled: true,
		OIDCSubject: &sub, OIDCIssuer: &issuer,
	})

	svc.userRoles = &errUserRoleRepo{
		UserRoleRepository: svc.userRoles,
		getErr:             errors.New("roles db boom"),
	}

	var (
		accessTk, refreshTk string
		err                 error
	)
	logs := captureSlogDuring(t, func() {
		accessTk, refreshTk, _, err = svc.HandleCallback(context.Background(), "auth-code", "")
	})
	if err != nil {
		t.Fatalf("expected success despite roles error, got %v", err)
	}
	if accessTk == "" || refreshTk == "" {
		t.Error("expected non-empty tokens")
	}
	if !strings.Contains(logs, "oidc: get roles") {
		t.Errorf("expected 'oidc: get roles' warning, got: %s", logs)
	}
}

// Was ..._ContinuesEmpty, asserting the callback completed with an empty
// default profile. That tolerated degradation WAS a parental bypass: an empty
// ProfileID is what every gate reads as "no restriction", so an SSO login
// during a profile-repo fault handed out an unrestricted token. Fails closed
// now, like the password and token-generation paths.
func TestHandleCallback_ProfilesGetByUserIDError_FailsClosed(t *testing.T) {
	sub := "sub-puserr"
	svc, oidcSrv := setupCallbackSvc(t, map[string]any{
		"sub":                sub,
		"email":              "u@example.com",
		"preferred_username": "u",
	})
	defer oidcSrv.close()

	userRepo := svc.userRepo.(*userfakes.FakeUserRepository)
	issuer := oidcSrv.issuer
	userRepo.Create(context.Background(), &domainuser.User{
		ID: "usr-puserr", Username: "u", Enabled: true,
		OIDCSubject: &sub, OIDCIssuer: &issuer,
	})

	svc.profiles = &errProfileRepo{
		ProfileRepository: svc.profiles,
		getByUserErr:      errors.New("profiles boom"),
	}

	accessTk, _, _, err := svc.HandleCallback(context.Background(), "auth-code", "")
	if err == nil {
		t.Fatalf("a profile lookup failure must refuse the callback, got token %q", accessTk)
	}
	if accessTk != "" {
		t.Errorf("no token may be issued when the profile lookup failed, got %q", accessTk)
	}
}

// L210: defaultProfileID == "" && len(profs) > 0 -> falls back to first profile
// Already covered by TestHandleCallback_ProfileWithRoles but add explicit assertion via empty-profs branch.
func TestHandleCallback_NoProfiles_NoDefault(t *testing.T) {
	sub := "sub-nop"
	svc, oidcSrv := setupCallbackSvc(t, map[string]any{
		"sub":                sub,
		"email":              "u@example.com",
		"preferred_username": "u",
	})
	defer oidcSrv.close()

	userRepo := svc.userRepo.(*userfakes.FakeUserRepository)
	issuer := oidcSrv.issuer
	userRepo.Create(context.Background(), &domainuser.User{
		ID: "usr-nop", Username: "u", Enabled: true,
		OIDCSubject: &sub, OIDCIssuer: &issuer,
	})

	accessTk, _, _, err := svc.HandleCallback(context.Background(), "auth-code", "")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if accessTk == "" {
		t.Error("expected access token")
	}
}

// L210: when no IsDefault profile but profiles exist, fallback to first profile's ID.
func TestHandleCallback_NoDefaultProfile_FallsBackToFirst(t *testing.T) {
	sub := "sub-fallbackprf"
	svc, oidcSrv := setupCallbackSvc(t, map[string]any{
		"sub":                sub,
		"email":              "u@example.com",
		"preferred_username": "u",
	})
	defer oidcSrv.close()

	userRepo := svc.userRepo.(*userfakes.FakeUserRepository)
	issuer := oidcSrv.issuer
	userRepo.Create(context.Background(), &domainuser.User{
		ID: "usr-fbprf", Username: "u", Enabled: true,
		OIDCSubject: &sub, OIDCIssuer: &issuer,
	})
	profileRepo := svc.profiles.(*userfakes.FakeProfileRepository)
	profileRepo.Create(context.Background(), &domainuser.Profile{
		ID: "prf-only", UserID: "usr-fbprf", Name: "u", IsDefault: false, Locale: "en-US",
	})

	accessTk, _, _, err := svc.HandleCallback(context.Background(), "auth-code", "")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	tk := svc.tokens.(*userfakes.FakeTokenGenerator)
	claims, err := tk.ValidateAccessToken(accessTk)
	if err != nil {
		t.Fatalf("validate token: %v", err)
	}
	if claims.ProfileID != "prf-only" {
		t.Errorf("ProfileID = %q, want 'prf-only' (fallback to first non-default)", claims.ProfileID)
	}
}

// L217: profiles.GetByID errors -> oidcLocale stays empty, callback continues
func TestHandleCallback_ProfileGetByIDError_ContinuesEmptyLocale(t *testing.T) {
	sub := "sub-pgerr"
	svc, oidcSrv := setupCallbackSvc(t, map[string]any{
		"sub":                sub,
		"email":              "u@example.com",
		"preferred_username": "u",
	})
	defer oidcSrv.close()

	userRepo := svc.userRepo.(*userfakes.FakeUserRepository)
	issuer := oidcSrv.issuer
	userRepo.Create(context.Background(), &domainuser.User{
		ID: "usr-pgerr", Username: "u", Enabled: true,
		OIDCSubject: &sub, OIDCIssuer: &issuer,
	})

	// Create a default profile so defaultProfileID is set, then wrap GetByID to fail.
	innerProfiles := svc.profiles
	innerProfiles.Create(context.Background(), &domainuser.Profile{
		ID: "prf-pgerr", UserID: "usr-pgerr", Name: "u", IsDefault: true,
	})
	svc.profiles = &errProfileRepo{
		ProfileRepository: innerProfiles,
		getByIDErr:        errors.New("profile get boom"),
	}

	accessTk, _, _, err := svc.HandleCallback(context.Background(), "auth-code", "")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if accessTk == "" {
		t.Error("expected access token")
	}
}

// L239: events.Publish (LoginSucceeded) errors -> slog.Warn, callback succeeds
func TestHandleCallback_LoginEventPublishError_Continues(t *testing.T) {
	sub := "sub-everr"
	svc, oidcSrv := setupCallbackSvc(t, map[string]any{
		"sub":                sub,
		"email":              "u@example.com",
		"preferred_username": "u",
	})
	defer oidcSrv.close()

	userRepo := svc.userRepo.(*userfakes.FakeUserRepository)
	issuer := oidcSrv.issuer
	userRepo.Create(context.Background(), &domainuser.User{
		ID: "usr-everr", Username: "u", Enabled: true,
		OIDCSubject: &sub, OIDCIssuer: &issuer,
	})

	bus := svc.events.(*sharedfakes.FakeEventBus)
	bus.PublishErr = errors.New("bus down")

	var (
		accessTk, refreshTk string
		err                 error
	)
	logs := captureSlogDuring(t, func() {
		accessTk, refreshTk, _, err = svc.HandleCallback(context.Background(), "auth-code", "")
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if accessTk == "" || refreshTk == "" {
		t.Error("expected non-empty tokens")
	}
	if !strings.Contains(logs, "oidc: publish event") {
		t.Errorf("expected 'oidc: publish event' warning, got: %s", logs)
	}
}

// This used to assert that a failing Update on the login path only produced a
// warning. That write is gone (see findOrCreateUser), so the assertion is now
// the stronger one: a repository whose Update ALWAYS fails must not perturb an
// OIDC login at all, because a login writes no user row.
func TestHandleCallback_ExistingUser_LoginPathWritesNoUserRow(t *testing.T) {
	sub := "sub-upderr"
	svc, oidcSrv := setupCallbackSvc(t, map[string]any{
		"sub":                sub,
		"email":              "u@example.com",
		"preferred_username": "u",
	})
	defer oidcSrv.close()

	inner := svc.userRepo.(*userfakes.FakeUserRepository)
	issuer := oidcSrv.issuer
	inner.Create(context.Background(), &domainuser.User{
		ID: "usr-upderr", Username: "u", Enabled: true,
		OIDCSubject: &sub, OIDCIssuer: &issuer,
	})
	svc.userRepo = &errUserRepo{UserRepository: inner, updateErr: errors.New("update boom")}

	var (
		accessTk string
		err      error
	)
	logs := captureSlogDuring(t, func() {
		accessTk, _, _, err = svc.HandleCallback(context.Background(), "auth-code", "")
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if accessTk == "" {
		t.Error("expected access token")
	}
	if strings.Contains(logs, "update user timestamp") {
		t.Errorf("the login path must not write the user row at all, got: %s", logs)
	}
}

// L266-268: email with "@" at position 0 -> at > 0 is false -> fallback hex username
func TestHandleCallback_EmailAtPositionZero_UsesHexFallback(t *testing.T) {
	sub := "sub-atzero"
	svc, oidcSrv := setupCallbackSvc(t, map[string]any{
		"sub":   sub,
		"email": "@example.com", // "@" at index 0
	})
	defer oidcSrv.close()

	_, _, _, err := svc.HandleCallback(context.Background(), "auth-code", "")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	userRepo := svc.userRepo.(*userfakes.FakeUserRepository)
	users, _ := userRepo.List(context.Background())
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
	if !strings.HasPrefix(users[0].Username, "oidc-") {
		t.Errorf("username = %q, want prefix 'oidc-'", users[0].Username)
	}
}

// profiles.Create error on first OIDC login now PROPAGATES (was swallowed with
// a warn-and-continue). The user + default profile create is wrapped in one tx,
// so a profile failure aborts the whole callback - and in production (real uow)
// rolls back the orphan user - instead of minting a token for a user that has
// no default profile (the GetByOIDCSubject fast-path would never heal it).
func TestHandleCallback_NewUserProfileCreateError_Propagates(t *testing.T) {
	svc, oidcSrv := setupCallbackSvc(t, map[string]any{
		"sub":                "sub-pcreaterr",
		"email":              "new@example.com",
		"preferred_username": "new",
	})
	defer oidcSrv.close()

	svc.profiles = &errProfileRepo{
		ProfileRepository: svc.profiles,
		createErr:         errors.New("profile create boom"),
	}

	accessTk, _, _, err := svc.HandleCallback(context.Background(), "auth-code", "")
	if err == nil {
		t.Fatal("expected the profile-create failure to propagate, got success")
	}
	if !strings.Contains(err.Error(), "default profile") {
		t.Errorf("error %q should surface the default profile failure", err)
	}
	if accessTk != "" {
		t.Error("no access token should be minted when the callback fails")
	}
}

// L305: new-user user.created publish error -> slog.Warn, callback succeeds
func TestHandleCallback_NewUserCreatedEventError_Continues(t *testing.T) {
	svc, oidcSrv := setupCallbackSvc(t, map[string]any{
		"sub":                "sub-newev",
		"email":              "newev@example.com",
		"preferred_username": "newev",
	})
	defer oidcSrv.close()

	bus := svc.events.(*sharedfakes.FakeEventBus)
	bus.PublishErr = errors.New("bus down")

	var (
		accessTk string
		err      error
	)
	logs := captureSlogDuring(t, func() {
		accessTk, _, _, err = svc.HandleCallback(context.Background(), "auth-code", "")
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if accessTk == "" {
		t.Error("expected access token")
	}
	if !strings.Contains(logs, "oidc: publish user.created") {
		t.Errorf("expected publish user.created warning, got: %s", logs)
	}
}

// L216: defaultProfileID != "" true branch -> Locale propagates into access token claims.
func TestHandleCallback_LocaleFromDefaultProfile(t *testing.T) {
	sub := "sub-locale"
	svc, oidcSrv := setupCallbackSvc(t, map[string]any{
		"sub":                sub,
		"email":              "u@example.com",
		"preferred_username": "u",
	})
	defer oidcSrv.close()

	userRepo := svc.userRepo.(*userfakes.FakeUserRepository)
	issuer := oidcSrv.issuer
	userRepo.Create(context.Background(), &domainuser.User{
		ID: "usr-locale", Username: "u", Enabled: true,
		OIDCSubject: &sub, OIDCIssuer: &issuer,
	})
	profileRepo := svc.profiles.(*userfakes.FakeProfileRepository)
	profileRepo.Create(context.Background(), &domainuser.Profile{
		ID: "prf-locale", UserID: "usr-locale", Name: "u", IsDefault: true, Locale: "fr-FR",
	})

	accessTk, _, _, err := svc.HandleCallback(context.Background(), "auth-code", "")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	tk := svc.tokens.(*userfakes.FakeTokenGenerator)
	claims, err := tk.ValidateAccessToken(accessTk)
	if err != nil {
		t.Fatalf("validate token: %v", err)
	}
	if claims.Locale != "fr-FR" {
		t.Errorf("Locale = %q, want 'fr-FR'", claims.Locale)
	}
	if claims.ProfileID != "prf-locale" {
		t.Errorf("ProfileID = %q, want 'prf-locale'", claims.ProfileID)
	}
}

// L148 boundary: time.Since > oidcStateTTL, test just inside and just outside
func TestValidateState_BoundaryExact(t *testing.T) {
	oidcSrv := newTestOIDCServer(t)
	defer oidcSrv.close()
	svc := buildOIDCService(t, oidcSrv)

	// Just outside TTL (1s older than the TTL) -> must be expired.
	ts := strconv.FormatInt(time.Now().Add(-oidcStateTTL-1*time.Second).Unix(), 10)
	payload := ts + "|n1"
	sig := svc.signState(payload)
	if _, err := svc.ValidateState(payload + "." + sig); !errors.Is(err, ErrStateExpired) {
		t.Errorf("just-past TTL: expected ErrStateExpired, got %v", err)
	}

	// Well inside TTL -> valid.
	ts2 := strconv.FormatInt(time.Now().Add(-1*time.Second).Unix(), 10)
	payload2 := ts2 + "|n2"
	sig2 := svc.signState(payload2)
	if _, err := svc.ValidateState(payload2 + "." + sig2); err != nil {
		t.Errorf("well-within TTL: expected valid, got %v", err)
	}
}

// --- OIDC nonce (id_token replay/injection defence) ---

func TestAuthURL_IncludesNonce(t *testing.T) {
	oidcSrv := newTestOIDCServer(t)
	defer oidcSrv.close()
	svc := buildOIDCService(t, oidcSrv)

	raw := svc.AuthURL("my-state-nonce")
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse AuthURL: %v", err)
	}
	if got := u.Query().Get("nonce"); got != "my-state-nonce" {
		t.Fatalf("AuthURL nonce = %q, want %q", got, "my-state-nonce")
	}
}

func TestHandleCallback_NonceMatch_OK(t *testing.T) {
	sub := "sub-nonce-ok"
	svc, oidcSrv := setupCallbackSvc(t, map[string]any{
		"sub": sub, "email": "n@example.com", "preferred_username": "n", "nonce": "good-nonce",
	})
	defer oidcSrv.close()

	issuer := oidcSrv.issuer
	svc.userRepo.(*userfakes.FakeUserRepository).Create(context.Background(), &domainuser.User{
		ID: "usr-nonce-ok", Username: "n", Enabled: true, OIDCSubject: &sub, OIDCIssuer: &issuer,
	})

	at, rt, _, err := svc.HandleCallback(context.Background(), "auth-code", "good-nonce")
	if err != nil {
		t.Fatalf("expected success with matching nonce, got %v", err)
	}
	if at == "" || rt == "" {
		t.Error("expected non-empty tokens")
	}
}

func TestHandleCallback_NonceMismatch_Rejected(t *testing.T) {
	svc, oidcSrv := setupCallbackSvc(t, map[string]any{
		"sub": "sub-nonce-bad", "email": "n@example.com", "nonce": "token-nonce",
	})
	defer oidcSrv.close()

	_, _, _, err := svc.HandleCallback(context.Background(), "auth-code", "different-nonce")
	if err == nil || !strings.Contains(err.Error(), "nonce mismatch") {
		t.Fatalf("expected nonce mismatch rejection, got %v", err)
	}
}

func TestHandleCallback_MissingNonceWhenExpected_Rejected(t *testing.T) {
	// id_token carries no nonce claim, but we expected one - must be rejected.
	svc, oidcSrv := setupCallbackSvc(t, map[string]any{
		"sub": "sub-nonce-missing", "email": "n@example.com",
	})
	defer oidcSrv.close()

	_, _, _, err := svc.HandleCallback(context.Background(), "auth-code", "expected-nonce")
	if err == nil || !strings.Contains(err.Error(), "nonce mismatch") {
		t.Fatalf("expected nonce mismatch for missing token nonce, got %v", err)
	}
}

// --- email_verified enforcement under a domain allowlist ---

func TestHandleCallback_UnverifiedEmail_RejectedUnderAllowlist(t *testing.T) {
	svc, oidcSrv := setupCallbackSvc(t, map[string]any{
		"sub": "sub-unverif", "email": "u@example.com", "email_verified": false, "nonce": "n1",
	}, func(s *OIDCService) { s.allowedEmailDomains = []string{"example.com"} })
	defer oidcSrv.close()

	_, _, _, err := svc.HandleCallback(context.Background(), "auth-code", "n1")
	if err == nil || !errors.Is(err, ErrEmailDomainNotAllowed) {
		t.Fatalf("expected rejection of unverified email under allowlist, got %v", err)
	}
}

func TestHandleCallback_VerifiedEmail_AllowedUnderAllowlist(t *testing.T) {
	svc, oidcSrv := setupCallbackSvc(t, map[string]any{
		"sub": "sub-verif", "email": "u@example.com", "email_verified": true, "preferred_username": "u", "nonce": "n2",
	}, func(s *OIDCService) { s.allowedEmailDomains = []string{"example.com"} })
	defer oidcSrv.close()

	at, _, _, err := svc.HandleCallback(context.Background(), "auth-code", "n2")
	if err != nil {
		t.Fatalf("expected success for verified email in allowed domain, got %v", err)
	}
	if at == "" {
		t.Error("expected non-empty access token")
	}
}

// --- an OIDC login must not rewrite the user row ---

// findOrCreateUser used to stamp UpdatedAt and persist through userRepo.Update,
// a blind 12-column write with no concurrency control. Replaying a snapshot read
// microseconds earlier over password_hash, is_admin and enabled means a login
// silently undoes whatever committed in between:
//
//   - a password reset (its own transaction already committed, its token burned
//     and refresh tokens purged) has the OLD hash restored;
//   - an admin's revocation of is_admin or enabled is reverted by the user's
//     next login, which they control and can repeat at will.
//
// Nothing was gained: HandleCallback calls TouchLastLogin (one column) right
// after, and that is what records a login.
// racingUserRepo lets an admin action land in the window between the handler's
// read and its write - the interval the whole hazard lives in, made
// deterministic instead of hopeful.
type racingUserRepo struct {
	domainuser.UserRepository
	inWindow func()
}

func (r *racingUserRepo) GetByOIDCSubject(ctx context.Context, issuer, subject string) (*domainuser.User, error) {
	u, err := r.UserRepository.GetByOIDCSubject(ctx, issuer, subject)
	if err == nil && r.inWindow != nil {
		hook := r.inWindow
		r.inWindow = nil // once: the admin acts a single time
		hook()
	}
	return u, err
}

func TestFindOrCreateUser_ExistingUser_DoesNotReplayTheRowItRead(t *testing.T) {
	ctx := context.Background()
	fake := userfakes.NewFakeUserRepository()
	sub, issuer := "sub-known", "https://idp.test"
	u, newErr := domainuser.NewUser("known", "known@example.com", "")
	if newErr != nil {
		t.Fatal(newErr)
	}
	u.OIDCSubject = &sub
	u.OIDCIssuer = &issuer
	u.PasswordHash = "old-hash"
	u.IsAdmin = true
	if err := fake.Create(ctx, u); err != nil {
		t.Fatal(err)
	}

	// Between the login's read and its write: the user completes a password
	// reset and an admin demotes them. Both are committed.
	repo := &racingUserRepo{UserRepository: fake}
	repo.inWindow = func() {
		cur, err := fake.GetByOIDCSubject(ctx, issuer, sub)
		if err != nil {
			t.Error(err)
			return
		}
		cur.PasswordHash = "new-hash-from-reset"
		cur.IsAdmin = false
		if err := fake.Update(ctx, cur); err != nil {
			t.Error(err)
		}
	}

	svc := &OIDCService{userRepo: repo, issuer: issuer}
	if _, err := svc.findOrCreateUser(ctx, sub, "known@example.com", "known", true); err != nil {
		t.Fatalf("findOrCreateUser: %v", err)
	}

	stored, err := fake.GetByOIDCSubject(ctx, issuer, sub)
	if err != nil {
		t.Fatal(err)
	}
	if stored.PasswordHash != "new-hash-from-reset" {
		t.Errorf("password_hash = %q, want the reset one: a login restored the old password, "+
			"while the reset token is already burned and the refresh tokens purged", stored.PasswordHash)
	}
	if stored.IsAdmin {
		t.Error("is_admin came back: a user can undo their own demotion by logging in, as often as they like")
	}
}

// The disabled check must still fail closed, and must do so before any write.
func TestFindOrCreateUser_DisabledUser_StillRefused(t *testing.T) {
	repo := userfakes.NewFakeUserRepository()
	sub, issuer := "sub-disabled", "https://idp.test"
	u, err := domainuser.NewUser("off", "off@example.com", "")
	if err != nil {
		t.Fatal(err)
	}
	u.OIDCSubject = &sub
	u.OIDCIssuer = &issuer
	u.Enabled = false
	if err := repo.Create(context.Background(), u); err != nil {
		t.Fatal(err)
	}

	svc := &OIDCService{userRepo: repo, issuer: issuer}
	if _, err := svc.findOrCreateUser(context.Background(), sub, "off@example.com", "off", true); !errors.Is(err, ErrUserDisabled) {
		t.Fatalf("expected ErrUserDisabled, got %v", err)
	}
}
