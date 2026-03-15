package user

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"golang.org/x/oauth2"

	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	userfakes "github.com/gravity-zero/evey/internal/domain/user/fakes"
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
	doc := map[string]interface{}{
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

func (s *testOIDCServer) signIDToken(t *testing.T, claims map[string]interface{}, clientID string) string {
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
	json.NewEncoder(w).Encode(map[string]interface{}{
		"access_token": "test-access-token",
		"token_type":   "Bearer",
		"expires_in":   3600,
	})
}

func (s *testOIDCServer) handleTokenWithIDToken(idToken string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
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
	if !strings.Contains(url, "my-nonce.") {
		t.Errorf("URL should contain nonce with signature separator, got %s", url)
	}
}

func TestValidateState_ValidRoundTrip(t *testing.T) {
	oidcSrv := newTestOIDCServer(t)
	defer oidcSrv.close()
	svc := buildOIDCService(t, oidcSrv)

	signed := svc.signState("nonce123")
	nonce, err := svc.ValidateState("nonce123." + signed)
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

	_, err := svc.ValidateState("nonce.wrong-signature")
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

	sig := svc1.signState("nonce")
	_, err := svc2.ValidateState("nonce." + sig)
	if err == nil {
		t.Fatal("expected mismatch with different secrets")
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

	_, _, err := svc.HandleCallback(context.Background(), "auth-code")
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
	idToken := oidcSrv.signIDToken(t, map[string]interface{}{
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
	mux.Handle("/token", http.HandlerFunc(oidcSrv2.handleTokenWithIDToken(
		oidcSrv2.signIDToken(t, map[string]interface{}{
			"sub":                "oidc-sub-123",
			"email":              "alice@example.com",
			"preferred_username": "alice",
			"name":               "Alice",
		}, "test-client"),
	)))
	oidcSrv2.srv.Config.Handler = mux

	svc := buildOIDCService(t, oidcSrv2)
	userRepo := svc.userRepo.(*userfakes.FakeUserRepository)
	events := svc.events.(*sharedfakes.FakeEventBus)

	accessTk, refreshTk, err := svc.HandleCallback(context.Background(), "auth-code")
	if err != nil {
		t.Fatalf("HandleCallback failed: %v", err)
	}
	if accessTk == "" {
		t.Error("expected non-empty access token")
	}
	if refreshTk == "" {
		t.Error("expected non-empty refresh token")
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
	mux.Handle("/token", http.HandlerFunc(oidcSrv.handleTokenWithIDToken(
		oidcSrv.signIDToken(t, map[string]interface{}{
			"sub":                sub,
			"email":              "bob@example.com",
			"preferred_username": "bob",
		}, "test-client"),
	)))
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

	accessTk, refreshTk, err := svc.HandleCallback(context.Background(), "auth-code")
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
	mux.Handle("/token", http.HandlerFunc(oidcSrv.handleTokenWithIDToken(
		oidcSrv.signIDToken(t, map[string]interface{}{
			"sub":   sub,
			"email": "disabled@example.com",
		}, "test-client"),
	)))
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

	_, _, err := svc.HandleCallback(context.Background(), "auth-code")
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
	mux.Handle("/token", http.HandlerFunc(oidcSrv.handleTokenWithIDToken(
		oidcSrv.signIDToken(t, map[string]interface{}{
			"sub":                "new-sub",
			"email":              "user@evil.com",
			"preferred_username": "hacker",
		}, "test-client"),
	)))
	oidcSrv.srv.Config.Handler = mux

	svc := buildOIDCService(t, oidcSrv, func(s *OIDCService) {
		s.allowedEmailDomains = []string{"example.com"}
	})

	_, _, err := svc.HandleCallback(context.Background(), "auth-code")
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
	mux.Handle("/token", http.HandlerFunc(oidcSrv.handleTokenWithIDToken(
		oidcSrv.signIDToken(t, map[string]interface{}{
			"sub":                "sub-tknerr",
			"email":              "user@example.com",
			"preferred_username": "user",
		}, "test-client"),
	)))
	oidcSrv.srv.Config.Handler = mux

	tokenGen := userfakes.NewFakeTokenGenerator()
	svc := buildOIDCService(t, oidcSrv, func(s *OIDCService) {
		s.tokens = tokenGen
	})

	// Make GenerateAccessToken fail via Err field — but we need it to succeed for refresh.
	// Instead, use a custom token generator
	tokenGen.Err = errors.New("token gen boom")

	_, _, err := svc.HandleCallback(context.Background(), "auth-code")
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
	mux.Handle("/token", http.HandlerFunc(oidcSrv.handleTokenWithIDToken(
		oidcSrv.signIDToken(t, map[string]interface{}{
			"sub":                "sub-repoerr",
			"email":              "user@example.com",
			"preferred_username": "user",
		}, "test-client"),
	)))
	oidcSrv.srv.Config.Handler = mux

	svc := buildOIDCService(t, oidcSrv, func(s *OIDCService) {
		repo := userfakes.NewFakeUserRepository()
		repo.Err = errors.New("db connection lost")
		s.userRepo = repo
	})

	_, _, err := svc.HandleCallback(context.Background(), "auth-code")
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
	mux.Handle("/token", http.HandlerFunc(oidcSrv.handleTokenWithIDToken(
		oidcSrv.signIDToken(t, map[string]interface{}{
			"sub":   "sub-nousername",
			"email": "jane@example.com",
		}, "test-client"),
	)))
	oidcSrv.srv.Config.Handler = mux

	svc := buildOIDCService(t, oidcSrv)
	userRepo := svc.userRepo.(*userfakes.FakeUserRepository)

	_, _, err := svc.HandleCallback(context.Background(), "auth-code")
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
	mux.Handle("/token", http.HandlerFunc(oidcSrv.handleTokenWithIDToken(
		oidcSrv.signIDToken(t, map[string]interface{}{
			"sub": "sub-noemail",
		}, "test-client"),
	)))
	oidcSrv.srv.Config.Handler = mux

	svc := buildOIDCService(t, oidcSrv)
	userRepo := svc.userRepo.(*userfakes.FakeUserRepository)

	_, _, err := svc.HandleCallback(context.Background(), "auth-code")
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
	mux.Handle("/token", http.HandlerFunc(oidcSrv.handleTokenWithIDToken(
		oidcSrv.signIDToken(t, map[string]interface{}{
			"sub":                sub,
			"email":              "admin@example.com",
			"preferred_username": "admin",
		}, "test-client"),
	)))
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

	accessTk, _, err := svc.HandleCallback(context.Background(), "auth-code")
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
	mux.Handle("/token", http.HandlerFunc(oidcSrv.handleTokenWithIDToken(
		oidcSrv.signIDToken(t, map[string]interface{}{
			"sub":                "sub-refresherr",
			"email":              "user@example.com",
			"preferred_username": "user",
		}, "test-client"),
	)))
	oidcSrv.srv.Config.Handler = mux

	tokenGen := userfakes.NewFakeTokenGenerator()
	svc := buildOIDCService(t, oidcSrv, func(s *OIDCService) {
		s.tokens = tokenGen
	})

	// Use a wrapper that fails only on refresh token generation
	svc.tokens = &failOnRefreshTokenGen{
		delegate:       tokenGen,
		refreshTokenErr: errors.New("refresh gen boom"),
	}

	_, _, err := svc.HandleCallback(context.Background(), "auth-code")
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

	_, _, err := svc.HandleCallback(context.Background(), "bad-code")
	if err == nil {
		t.Fatal("expected error on token exchange failure")
	}
	if !strings.Contains(err.Error(), "token exchange") {
		t.Errorf("error should mention 'token exchange', got %q", err.Error())
	}
}

// --- Ping ---

func TestPing_Success(t *testing.T) {
	oidcSrv := newTestOIDCServer(t)
	defer oidcSrv.close()
	svc := buildOIDCService(t, oidcSrv)

	if err := svc.Ping(context.Background()); err != nil {
		t.Fatalf("Ping should succeed: %v", err)
	}
}

func TestPing_IssuerMismatch(t *testing.T) {
	oidcSrv := newTestOIDCServer(t)
	defer oidcSrv.close()
	svc := buildOIDCService(t, oidcSrv, func(s *OIDCService) {
		s.issuer = "http://wrong-issuer.example.com"
	})

	err := svc.Ping(context.Background())
	if err == nil {
		t.Fatal("expected error for issuer mismatch")
	}
}

// --- generateAndStoreRefreshToken ---

func TestGenerateAndStoreRefreshToken_DuplicateHashRetries(t *testing.T) {
	oidcSrv := newTestOIDCServer(t)
	defer oidcSrv.close()
	svc := buildOIDCService(t, oidcSrv)

	// Should succeed normally
	token, err := svc.generateAndStoreRefreshToken(context.Background(), "usr-123")
	if err != nil {
		t.Fatalf("generateAndStoreRefreshToken failed: %v", err)
	}
	if token == "" {
		t.Error("expected non-empty token")
	}
}

func TestGenerateAndStoreRefreshToken_PermanentError(t *testing.T) {
	oidcSrv := newTestOIDCServer(t)
	defer oidcSrv.close()

	refreshRepo := userfakes.NewFakeRefreshTokenRepository()
	refreshRepo.Err = errors.New("db down")
	svc := buildOIDCService(t, oidcSrv, func(s *OIDCService) {
		s.refreshTkns = refreshRepo
	})

	_, err := svc.generateAndStoreRefreshToken(context.Background(), "usr-123")
	if err == nil {
		t.Fatal("expected error when repo always fails")
	}
	if !strings.Contains(err.Error(), "duplicate hash after retries") {
		t.Errorf("expected 'duplicate hash after retries', got %q", err.Error())
	}
}

