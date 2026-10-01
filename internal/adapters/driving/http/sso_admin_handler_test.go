package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	appconfig "github.com/evey-project/evey/internal/app/config"
	appuser "github.com/evey-project/evey/internal/app/user"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
	"github.com/evey-project/evey/internal/pkg/auth"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
)

type fakeLibLookup struct{}

func (fakeLibLookup) GetLibrary(_ context.Context, id string) (*domainlibrary.Library, error) {
	if id == "lib-1" {
		return &domainlibrary.Library{}, nil
	}
	return nil, errors.New("not found")
}

type ssoFixture struct {
	mux      *http.ServeMux
	store    *sharedfakes.FakeServerConfigRepository
	idp      *httptest.Server
	adminTok string
	userTok  string
}

// newSSOFixture wires the admin SSO routes and the public OIDC routes on one
// mux, over a live settings source, the way the server does.
func newSSOFixture(t *testing.T, env appuser.OIDCConfig) *ssoFixture {
	t.Helper()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	adminTok, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "admin-1", IsAdmin: true})
	userTok, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1"})
	store := sharedfakes.NewFakeServerConfigRepository()
	src := appuser.OIDCSettingsSource{Env: env, Store: store, BaseURL: func() string { return "https://evey.example" }}
	svc, err := appuser.NewOIDCService(src.Load,
		userfakes.NewFakeUserRepository(), userfakes.NewFakeProfileRepository(), userfakes.NewFakeRefreshTokenRepository(),
		userfakes.NewFakeUserRoleRepository(userfakes.NewFakeRoleRepository()), userfakes.NewFakeTokenGenerator(),
		sharedfakes.NewFakeEventBus(), nil, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	writeRL := ratelimit.New(1000)
	t.Cleanup(writeRL.Stop)
	mux := http.NewServeMux()
	registerOIDCRoutes(mux, svc, false)
	registerSSOAdminRoutes(mux, svc, appconfig.NewService(store), fakeLibLookup{}, gen, writeRL)
	return &ssoFixture{mux: mux, store: store, idp: newMockOIDCServer(t), adminTok: adminTok, userTok: userTok}
}

func (f *ssoFixture) do(t *testing.T, method, path, tok string, body any) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Host = "evey.example" // the server is reached on its configured base URL
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	out := map[string]any{}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w, out
}

func TestSSOAdmin_RoutesAreAdminOnly(t *testing.T) {
	f := newSSOFixture(t, appuser.OIDCConfig{})
	for _, rt := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/admin/sso"},
		{http.MethodPut, "/api/v1/admin/sso"},
		{http.MethodPost, "/api/v1/admin/sso/test"},
	} {
		if w, _ := f.do(t, rt.method, rt.path, "", map[string]any{}); w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s anonymous: %d, want 401", rt.method, rt.path, w.Code)
		}
		if w, _ := f.do(t, rt.method, rt.path, f.userTok, map[string]any{}); w.Code != http.StatusForbidden {
			t.Errorf("%s %s as a viewer: %d, want 403", rt.method, rt.path, w.Code)
		}
	}
	if storeLen(f.store) != 0 {
		t.Error("a refused PUT wrote settings")
	}
}

func TestSSOAdmin_GetMasksSecretAndShowsURIs(t *testing.T) {
	f := newSSOFixture(t, appuser.OIDCConfig{Enabled: true, Issuer: "https://idp.invalid", ClientID: "env-client", ClientSecret: "env-secret"})
	w, out := f.do(t, http.MethodGet, "/api/v1/admin/sso", f.adminTok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET: %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "env-secret") {
		t.Fatal("the client secret left the server in clear")
	}
	if out["clientSecret"] != appconfig.SecretMask || out["source"] != "env" {
		t.Errorf("secret=%v source=%v", out["clientSecret"], out["source"])
	}
	if out["redirectUri"] != "https://evey.example/api/v1/auth/oidc/callback" || out["postLogoutRedirectUri"] != "https://evey.example/login" {
		t.Errorf("uris = %v / %v", out["redirectUri"], out["postLogoutRedirectUri"])
	}
	if out["allowSignup"] != false {
		t.Error("no allowlist: sign-up must default off")
	}
}

func TestSSOAdmin_PutAppliesLiveAndKeepsMaskedSecret(t *testing.T) {
	f := newSSOFixture(t, appuser.OIDCConfig{})
	// Off at boot: the login page offers nothing.
	if _, cfg := f.do(t, http.MethodGet, "/api/v1/auth/oidc/config", "", nil); cfg["enabled"] != false {
		t.Fatalf("config before = %v", cfg)
	}
	w, out := f.do(t, http.MethodPut, "/api/v1/admin/sso", f.adminTok, map[string]any{
		"enabled": true, "issuer": f.idp.URL, "clientId": "evey", "clientSecret": "s3cret",
		"allowedEmailDomains": []string{"Example.com"}, "buttonLabel": "Corp login", "defaultLibraryIds": []string{"lib-1"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("PUT: %d %s", w.Code, w.Body.String())
	}
	status, _ := out["status"].(map[string]any)
	if status["state"] != "ok" {
		t.Fatalf("status after save = %v", status)
	}
	if out["clientSecret"] != appconfig.SecretMask || strings.Contains(w.Body.String(), "s3cret") {
		t.Error("secret echoed in clear")
	}
	if e, _ := f.store.Get(context.Background(), appuser.OIDCKeyClientSecret); e == nil || e.Value != "s3cret" {
		t.Fatalf("secret not stored: %+v", e)
	}
	if e, _ := f.store.Get(context.Background(), appuser.OIDCKeyAllowedEmailDomains); e.Value != "example.com" {
		t.Errorf("domains stored as %q", e.Value)
	}
	// No restart: the login page offers SSO with the label right away.
	if _, cfg := f.do(t, http.MethodGet, "/api/v1/auth/oidc/config", "", nil); cfg["enabled"] != true || cfg["buttonLabel"] != "Corp login" {
		t.Fatalf("config after save = %v", cfg)
	}

	// Saving the form again with the mask must not overwrite the secret.
	if w, _ := f.do(t, http.MethodPut, "/api/v1/admin/sso", f.adminTok, map[string]any{"clientSecret": appconfig.SecretMask, "enabled": false}); w.Code != http.StatusOK {
		t.Fatalf("PUT 2: %d", w.Code)
	}
	if e, _ := f.store.Get(context.Background(), appuser.OIDCKeyClientSecret); e.Value != "s3cret" {
		t.Fatalf("mask overwrote the secret: %q", e.Value)
	}
	if _, cfg := f.do(t, http.MethodGet, "/api/v1/auth/oidc/config", "", nil); cfg["enabled"] != false {
		t.Fatalf("disabling did not apply live: %v", cfg)
	}
}

func TestSSOAdmin_PutValidates(t *testing.T) {
	f := newSSOFixture(t, appuser.OIDCConfig{})
	for name, body := range map[string]map[string]any{
		"relative issuer": {"issuer": "idp.example.com"},
		"issuer query":    {"issuer": "https://idp.example.com/?x=1"},
		"ftp issuer":      {"issuer": "ftp://idp.example.com"},
		"bad domain":      {"allowedEmailDomains": []string{"not a domain"}},
		"unknown library": {"defaultLibraryIds": []string{"lib-404"}},
		"long label":      {"buttonLabel": strings.Repeat("x", 65)},
	} {
		if w, _ := f.do(t, http.MethodPut, "/api/v1/admin/sso", f.adminTok, body); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", name, w.Code)
		}
	}
	if storeLen(f.store) != 0 {
		t.Error("an invalid PUT wrote settings")
	}
}

func TestSSOAdmin_TestConnection(t *testing.T) {
	f := newSSOFixture(t, appuser.OIDCConfig{})
	_, out := f.do(t, http.MethodPost, "/api/v1/admin/sso/test", f.adminTok, map[string]any{"issuer": f.idp.URL})
	if out["ok"] != true {
		t.Fatalf("probe of a live provider: %v", out)
	}
	bad := httptest.NewServer(http.NotFoundHandler())
	defer bad.Close()
	_, out = f.do(t, http.MethodPost, "/api/v1/admin/sso/test", f.adminTok, map[string]any{"issuer": bad.URL})
	if out["ok"] != false || out["error"] == "" {
		t.Fatalf("probe of a dead issuer: %v", out)
	}
}

func TestOIDCErrorReason(t *testing.T) {
	cases := map[error]string{
		appuser.ErrEmailDomainNotAllowed:  "domain_not_allowed",
		appuser.ErrOIDCSignupDisabled:     "signup_disabled",
		appuser.ErrUserDisabled:           "account_disabled",
		appuser.ErrOIDCAccountExists:      "account_exists",
		appuser.ErrOIDCUnavailable:        "unavailable",
		appuser.ErrStateExpired:           "expired",
		errors.New("token exchange: 500"): "failed",
	}
	for err, want := range cases {
		if got := oidcErrorReason(errors.Join(errors.New("wrapped"), err)); got != want {
			t.Errorf("%v -> %q, want %q", err, got, want)
		}
	}
}

func TestOIDCLogin_ProviderDownRedirectsWithReason(t *testing.T) {
	f := newSSOFixture(t, appuser.OIDCConfig{Enabled: true, Issuer: "http://127.0.0.1:1/dead", ClientID: "c", ClientSecret: "s"})
	w, _ := f.do(t, http.MethodGet, "/api/v1/auth/oidc/login", "", nil)
	if w.Code != http.StatusFound || w.Header().Get("Location") != "/login?oidc_error=unavailable" {
		t.Fatalf("login with a dead provider: %d %q", w.Code, w.Header().Get("Location"))
	}
	if _, cfg := f.do(t, http.MethodGet, "/api/v1/auth/oidc/config", "", nil); cfg["enabled"] != false {
		t.Error("button offered while the provider is down")
	}
}

func TestOIDCCallback_IdPCancelRedirectsWithReason(t *testing.T) {
	f := newSSOFixture(t, appuser.OIDCConfig{})
	w, _ := f.do(t, http.MethodGet, "/api/v1/auth/oidc/callback?error=access_denied&state=x", "", nil)
	if w.Header().Get("Location") != "/login?oidc_error=cancelled" {
		t.Fatalf("location = %q", w.Header().Get("Location"))
	}
}

func TestOIDCEndSessionURL(t *testing.T) {
	got := oidcEndSessionURL("https://idp/logout?client=x", "", "https://evey/login", "tok")
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("client") != "x" || q.Get("post_logout_redirect_uri") != "https://evey/login" || q.Get("id_token_hint") != "tok" {
		t.Errorf("end session url = %q", got)
	}
}

// --- mobile app flow ---

func TestOIDCAppFlows_NeedBothSecrets(t *testing.T) {
	f := newOIDCAppFlows()
	id, poll, ok, err := f.start()
	if err != nil || !ok {
		t.Fatal(err)
	}
	if !f.pending(id) || f.pending("unknown") {
		t.Fatal("pending wrong")
	}
	code, ok := f.complete(id, "acc", "ref")
	if !ok || f.pending(id) {
		t.Fatal("complete wrong")
	}
	if _, again := f.complete(id, "acc2", "ref2"); again {
		t.Fatal("a flow completed twice")
	}
	// The deep-link code alone (another app caught it) is worthless, and the
	// attempt burns the flow.
	if _, _, got := f.redeem(code, "guess"); got {
		t.Fatal("redeemed without the poll secret")
	}
	if _, _, got := f.redeem(code, poll); got {
		t.Fatal("a burnt flow was redeemed")
	}

	id2, poll2, _, _ := f.start()
	code2, _ := f.complete(id2, "acc", "ref")
	if _, _, got := f.redeem("other-code", poll2); got {
		t.Fatal("the poll secret alone (phishing link) redeemed a flow")
	}
	a, r, ok := f.redeem(code2, poll2)
	if !ok || a != "acc" || r != "ref" {
		t.Fatalf("legit redeem: %v %q %q", ok, a, r)
	}
	if _, _, ok := f.redeem(code2, poll2); ok {
		t.Fatal("redeemed twice")
	}
}

func TestOIDCAppFlows_Expire(t *testing.T) {
	f := newOIDCAppFlows()
	now := time.Now()
	f.now = func() time.Time { return now }
	id, poll, _, _ := f.start()
	code, _ := f.complete(id, "a", "r")
	now = now.Add(oidcAppFlowTTL + time.Second)
	if _, _, ok := f.redeem(code, poll); ok {
		t.Fatal("an expired flow was redeemed")
	}
	id3, _, _, _ := f.start()
	now = now.Add(oidcAppFlowTTL + time.Second)
	if f.pending(id3) || f.fail(id3) {
		t.Fatal("an expired flow is still pending")
	}
}

func TestOIDCAppStart_RoutesTheLoginThroughTheFlow(t *testing.T) {
	f := newSSOFixture(t, appuser.OIDCConfig{})
	if w, _ := f.do(t, http.MethodPost, "/api/v1/auth/oidc/app/start", "", nil); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("start with SSO off: %d", w.Code)
	}
	f.do(t, http.MethodPut, "/api/v1/admin/sso", f.adminTok, map[string]any{"enabled": true, "issuer": f.idp.URL, "clientId": "evey", "clientSecret": "s"})
	w, out := f.do(t, http.MethodPost, "/api/v1/auth/oidc/app/start", "", nil)
	if w.Code != http.StatusOK || out["pollToken"] == "" {
		t.Fatalf("start: %d %v", w.Code, out)
	}
	loginPath, _ := out["loginPath"].(string)
	flowID := strings.TrimPrefix(loginPath, "/api/v1/auth/oidc/login?flow=")
	w, _ = f.do(t, http.MethodGet, loginPath, "", nil)
	loc, _ := url.Parse(w.Header().Get("Location"))
	if w.Code != http.StatusFound || loc.Query().Get("nonce") != flowID {
		t.Fatalf("login of an app flow: %d %q", w.Code, w.Header().Get("Location"))
	}
	w, _ = f.do(t, http.MethodGet, "/api/v1/auth/oidc/login?flow=forged", "", nil)
	if w.Header().Get("Location") != "/login?oidc_error=expired" {
		t.Fatalf("forged flow id: %q", w.Header().Get("Location"))
	}
	// An IdP refusal on an app flow goes back to the app, not the web page.
	state := loc.Query().Get("state")
	w, _ = f.do(t, http.MethodGet, "/api/v1/auth/oidc/callback?error=access_denied&state="+url.QueryEscape(state), "", nil)
	if w.Header().Get("Location") != oidcAppRedirect+"?oidc_error=cancelled" {
		t.Fatalf("app flow refusal went to %q", w.Header().Get("Location"))
	}
	if w, _ := f.do(t, http.MethodPost, "/api/v1/auth/oidc/app/token", "", map[string]string{"code": "x", "pollToken": "y"}); w.Code != http.StatusBadRequest {
		t.Fatalf("bogus redeem: %d", w.Code)
	}
}

func storeLen(s *sharedfakes.FakeServerConfigRepository) int {
	all, _ := s.List(context.Background())
	return len(all)
}
