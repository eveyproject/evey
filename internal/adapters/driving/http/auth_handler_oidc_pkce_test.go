package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	appuser "github.com/eveyproject/evey/internal/app/user"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	userfakes "github.com/eveyproject/evey/internal/domain/user/fakes"
)

// pkceIdP is a fake provider that binds each code to the authorization
// request (nonce, challenge) and, like a real one, enforces PKCE only when a
// challenge was sent.
type pkceIdP struct {
	srv *httptest.Server
	key *rsa.PrivateKey

	// ignorePKCE plays a provider without PKCE support: only the browser
	// binding then stands between a planted code and a session.
	ignorePKCE bool

	mu       sync.Mutex
	codes    map[string]pkceGrant
	verifier []string // code_verifier received by each token call
}

type pkceGrant struct{ nonce, challenge, method, sub string }

func newPKCEIdP(t *testing.T) *pkceIdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	p := &pkceIdP{key: key, codes: map[string]pkceGrant{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": p.srv.URL, "authorization_endpoint": p.srv.URL + "/auth",
			"token_endpoint": p.srv.URL + "/token", "jwks_uri": p.srv.URL + "/jwks",
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("/token", p.token(t))
	p.srv = httptest.NewServer(mux)
	t.Cleanup(p.srv.Close)
	return p
}

// authorize plays the person signing in at the provider: it reads the
// authorization request from the login redirect and returns code + state.
func (p *pkceIdP) authorize(t *testing.T, location, sub string) (code, state string) {
	t.Helper()
	u, err := url.Parse(location)
	if err != nil || !strings.HasPrefix(location, p.srv.URL+"/auth") {
		t.Fatalf("login did not redirect to the provider: %q", location)
	}
	q := u.Query()
	code = "code-" + sub + "-" + q.Get("nonce")
	p.mu.Lock()
	p.codes[code] = pkceGrant{nonce: q.Get("nonce"), challenge: q.Get("code_challenge"), method: q.Get("code_challenge_method"), sub: sub}
	p.mu.Unlock()
	return code, q.Get("state")
}

func (p *pkceIdP) token(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		p.mu.Lock()
		g, ok := p.codes[r.PostForm.Get("code")]
		ver := r.PostForm.Get("code_verifier")
		p.verifier = append(p.verifier, ver)
		p.mu.Unlock()
		if !ok {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		if g.challenge != "" && !p.ignorePKCE {
			sum := sha256.Sum256([]byte(ver))
			if g.method != "S256" || base64.RawURLEncoding.EncodeToString(sum[:]) != g.challenge {
				http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
				return
			}
		}
		signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: p.key}, (&jose.SignerOptions{}).WithHeader("kid", "k1"))
		if err != nil {
			t.Error(err)
			return
		}
		now := time.Now()
		raw, err := jwt.Signed(signer).Claims(jwt.Claims{
			Issuer: p.srv.URL, Subject: g.sub, Audience: jwt.Audience{"evey"},
			IssuedAt: jwt.NewNumericDate(now), Expiry: jwt.NewNumericDate(now.Add(5 * time.Minute)),
		}).Claims(map[string]any{"nonce": g.nonce, "email": g.sub + "@example.com", "email_verified": true, "preferred_username": g.sub}).Serialize()
		if err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer", "expires_in": 60, "id_token": raw})
	}
}

func newPKCEFixture(t *testing.T) (*http.ServeMux, *pkceIdP) {
	t.Helper()
	idp := newPKCEIdP(t)
	cfg := appuser.OIDCConfig{
		Enabled: true, Issuer: idp.srv.URL, ClientID: "evey", ClientSecret: "s",
		RedirectURL: "https://example.com" + appuser.OIDCCallbackPath, AllowSignup: true,
	}
	svc, err := appuser.NewOIDCService(func(context.Context) (appuser.OIDCConfig, error) { return cfg, nil },
		userfakes.NewFakeUserRepository(), userfakes.NewFakeProfileRepository(), userfakes.NewFakeRefreshTokenRepository(),
		userfakes.NewFakeUserRoleRepository(userfakes.NewFakeRoleRepository()), userfakes.NewFakeTokenGenerator(),
		sharedfakes.NewFakeEventBus(), nil, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerOIDCRoutes(mux, svc, false)
	return mux, idp
}

func serve(mux *http.ServeMux, method, target string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func responseCookie(w *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range w.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// login runs GET /login and returns the provider redirect and the binding
// cookie the browser now holds (nil when none was set).
func login(t *testing.T, mux *http.ServeMux, target string) (string, *http.Cookie) {
	t.Helper()
	w := serve(mux, http.MethodGet, target)
	if w.Code != http.StatusFound {
		t.Fatalf("login: %d", w.Code)
	}
	return w.Header().Get("Location"), responseCookie(w, oidcLoginCookieName)
}

func callbackURL(code, state string) string {
	return "/api/v1/auth/oidc/callback?code=" + url.QueryEscape(code) + "&state=" + url.QueryEscape(state)
}

func TestOIDCLogin_PKCEAndBrowserBinding_FullFlow(t *testing.T) {
	mux, idp := newPKCEFixture(t)
	loc, cookie := login(t, mux, "/api/v1/auth/oidc/login")
	if cookie == nil {
		t.Fatal("login set no binding cookie: the state is tied to no browser")
	}
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != appuser.OIDCCallbackPath || cookie.MaxAge <= 0 || cookie.MaxAge > 600 {
		t.Errorf("binding cookie attributes: %+v", cookie)
	}
	if strings.Contains(loc, url.QueryEscape(cookie.Value)) || strings.Contains(loc, cookie.Value) {
		t.Fatal("the PKCE verifier leaked into the authorization URL")
	}
	u, _ := url.Parse(loc)
	if u.Query().Get("code_challenge_method") != "S256" || u.Query().Get("code_challenge") == "" {
		t.Fatalf("no S256 challenge in %q", loc)
	}

	code, state := idp.authorize(t, loc, "alice")
	w := serve(mux, http.MethodGet, callbackURL(code, state), cookie)
	if w.Code != http.StatusFound || w.Header().Get("Location") != "/oidc-callback" {
		t.Fatalf("callback: %d %q", w.Code, w.Header().Get("Location"))
	}
	if responseCookie(w, refreshCookieName) == nil {
		t.Fatal("no session after a legitimate login")
	}
	if c := responseCookie(w, oidcLoginCookieName); c == nil || c.MaxAge >= 0 {
		t.Error("binding cookie not cleared after the callback")
	}
	idp.mu.Lock()
	got := idp.verifier
	idp.mu.Unlock()
	sum := sha256.Sum256([]byte(cookie.Value))
	if len(got) != 1 || got[0] != cookie.Value || base64.RawURLEncoding.EncodeToString(sum[:]) != u.Query().Get("code_challenge") {
		t.Fatalf("token exchange code_verifier = %q, want the cookie verifier matching the challenge", got)
	}
}

// Login CSRF: the attacker signs in at the provider and plants his own code
// and valid state in the victim's browser, which holds no binding cookie.
func TestOIDCCallback_ForeignStateWithoutCookie_Refused(t *testing.T) {
	mux, idp := newPKCEFixture(t)
	idp.ignorePKCE = true
	loc, _ := login(t, mux, "/api/v1/auth/oidc/login")
	code, state := idp.authorize(t, loc, "attacker")

	w := serve(mux, http.MethodGet, callbackURL(code, state))
	if responseCookie(w, refreshCookieName) != nil || w.Header().Get("Location") == "/oidc-callback" {
		t.Fatal("a callback without the binding cookie signed the browser in")
	}
	if w.Code != http.StatusFound || !strings.HasPrefix(w.Header().Get("Location"), "/login?oidc_error=") {
		t.Fatalf("refusal: %d %q", w.Code, w.Header().Get("Location"))
	}
	idp.mu.Lock()
	calls := len(idp.verifier)
	idp.mu.Unlock()
	if calls != 0 {
		t.Error("the code was exchanged before the browser binding was checked")
	}
}

// Same attack on a victim who started a login of her own: her cookie does
// not match the attacker's state.
func TestOIDCCallback_ForeignStateWithAnotherLoginCookie_Refused(t *testing.T) {
	mux, idp := newPKCEFixture(t)
	idp.ignorePKCE = true
	attackerLoc, _ := login(t, mux, "/api/v1/auth/oidc/login")
	code, state := idp.authorize(t, attackerLoc, "attacker")
	_, victimCookie := login(t, mux, "/api/v1/auth/oidc/login")
	if victimCookie == nil {
		t.Fatal("login set no binding cookie")
	}

	w := serve(mux, http.MethodGet, callbackURL(code, state), victimCookie)
	if responseCookie(w, refreshCookieName) != nil || w.Header().Get("Location") == "/oidc-callback" {
		t.Fatal("another login's cookie validated the attacker's state")
	}
	idp.mu.Lock()
	calls := len(idp.verifier)
	idp.mu.Unlock()
	if calls != 0 {
		t.Error("the code was exchanged before the browser binding was checked")
	}
}

func TestOIDCAppFlow_BoundCallbackStillCompletes(t *testing.T) {
	mux, idp := newPKCEFixture(t)
	w := serve(mux, http.MethodPost, "/api/v1/auth/oidc/app/start")
	var start map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &start); err != nil || w.Code != http.StatusOK {
		t.Fatalf("app start: %d %s", w.Code, w.Body.String())
	}
	loc, cookie := login(t, mux, start["loginPath"])
	if cookie == nil {
		t.Fatal("app flow login set no binding cookie")
	}
	code, state := idp.authorize(t, loc, "bob")

	// Without the cookie: back to the app with an error, nothing to redeem.
	w = serve(mux, http.MethodGet, callbackURL(code, state))
	if !strings.HasPrefix(w.Header().Get("Location"), oidcAppRedirect+"?oidc_error=") {
		t.Fatalf("unbound app callback went to %q", w.Header().Get("Location"))
	}

	// A fresh flow, completed by the same browser (cookie present).
	w = serve(mux, http.MethodPost, "/api/v1/auth/oidc/app/start")
	_ = json.Unmarshal(w.Body.Bytes(), &start)
	loc, cookie = login(t, mux, start["loginPath"])
	code, state = idp.authorize(t, loc, "bob")
	w = serve(mux, http.MethodGet, callbackURL(code, state), cookie)
	deep, _ := url.Parse(w.Header().Get("Location"))
	if !strings.HasPrefix(w.Header().Get("Location"), oidcAppRedirect+"?code=") || deep.Query().Get("code") == "" {
		t.Fatalf("bound app callback went to %q", w.Header().Get("Location"))
	}
	body := strings.NewReader(`{"code":"` + deep.Query().Get("code") + `","pollToken":"` + start["pollToken"] + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oidc/app/token", body)
	rw := httptest.NewRecorder()
	mux.ServeHTTP(rw, req)
	var tok map[string]string
	if err := json.Unmarshal(rw.Body.Bytes(), &tok); err != nil || rw.Code != http.StatusOK || tok["accessToken"] == "" || tok["refreshToken"] == "" {
		t.Fatalf("app redeem: %d %s", rw.Code, rw.Body.String())
	}
}
