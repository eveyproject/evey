package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	appuser "github.com/evey-project/evey/internal/app/user"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
	"github.com/evey-project/evey/internal/pkg/auth"
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
	oidcCfg := appuser.OIDCConfig{
		Enabled:      true,
		Issuer:       oidcSrv.URL,
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		RedirectURL:  "http://localhost/callback",
	}
	svc, err := appuser.NewOIDCService(
		context.Background(), oidcCfg,
		userfakes.NewFakeUserRepository(),
		userfakes.NewFakeProfileRepository(),
		userfakes.NewFakeRefreshTokenRepository(),
		userfakes.NewFakeUserRoleRepository(userfakes.NewFakeRoleRepository()),
		userfakes.NewFakeTokenGenerator(),
		nil,
		nil,
		24*time.Hour,
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

	// Send callback with valid state but a bogus code - provider token exchange will fail,
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

// =====================================
// Parental - cross-profile IDOR, nil parental, rate-limit exhaustion
// =====================================

func TestParentalVerifyPIN_CrossProfileIDOR(t *testing.T) {
	mux, gen := newParentalMux(t)
	// user-1 authenticated with own profile, tries verify-pin on admin profile (different UserID)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", ProfileID: "prof-1", IsAdmin: false})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/profiles/prof-admin/parental/verify-pin", strings.NewReader(`{"pin":"1234"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d body=%s", w.Code, w.Body.String())
	}
	// Ensure response does NOT leak parental state (e.g. "valid" field)
	if strings.Contains(w.Body.String(), `"valid"`) {
		t.Errorf("cross-profile response leaked parental state: %s", w.Body.String())
	}
}

func TestParentalVerifyPIN_AdminAllowedCrossProfile(t *testing.T) {
	mux, gen := newParentalMux(t)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "admin-1", ProfileID: "prof-admin", IsAdmin: true})
	// Admin hits prof-1 (belongs to user-1) legitimately
	req := httptest.NewRequest(http.MethodPost, "/api/v1/profiles/prof-1/parental/verify-pin", strings.NewReader(`{"pin":"1234"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for admin cross-profile, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestParentalVerifyPIN_NilParental(t *testing.T) {
	// Profile without Parental config - must not nil-pointer crash; returns valid=true.
	mux, gen := newParentalMux(t)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", ProfileID: "prof-1", IsAdmin: false})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/profiles/prof-1/parental/verify-pin", strings.NewReader(`{"pin":""}`))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (no crash) for nil Parental, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"valid":true`) {
		t.Errorf("expected valid=true for nil parental, got %s", w.Body.String())
	}
}

func TestParentalVerifyPIN_RateLimitExhaustion(t *testing.T) {
	mux, gen := newParentalMux(t)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", ProfileID: "prof-1", IsAdmin: false})

	var got429 bool
	var successCount int
	// Burst is small (5 per production config); hammer until we see a 429.
	for i := 0; i < 50; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/profiles/prof-1/parental/verify-pin", strings.NewReader(`{"pin":"x"}`))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code == http.StatusTooManyRequests {
			got429 = true
			break
		}
		if w.Code == http.StatusOK {
			successCount++
		}
	}
	if !got429 {
		t.Fatalf("expected 429 after burst exhaustion, never got one (successes=%d)", successCount)
	}
	if successCount == 0 {
		t.Errorf("expected at least one successful request before RL kicks in, got 0")
	}
}

// newKidsCallerMux wires the parental routes over a repo holding BOTH a kids
// profile and a normal one, all owned by user-1 - so the caller and the target
// can be the same restricted profile, which is the escalation shape the gRPC
// gate blocks and the hand-written HTTP twin did not.
func newKidsCallerMux(t *testing.T) (*http.ServeMux, *auth.JWTGenerator, string) {
	t.Helper()
	gen, _, _ := genTokens(t)
	profileRepo := userfakes.NewFakeProfileRepository()
	maxAge := 10
	_ = profileRepo.Create(context.Background(), &domainuser.Profile{
		ID: "prof-kid", UserID: "user-1", Name: "Kid", IsKids: true,
		Parental: &domainuser.ParentalSettings{Enabled: true, MaxAge: &maxAge, BlockedTags: []string{"horror"}},
	})
	_ = profileRepo.Create(context.Background(), &domainuser.Profile{
		// Must hold a PIN: it is the non-kids sibling of prof-kid, and the
		// domain kids/PIN invariant (now enforced on every parental write,
		// not just Create) rejects a household missing that guardian PIN.
		ID: "prof-1", UserID: "user-1", Name: "Parent",
		Parental: &domainuser.ParentalSettings{PINHash: "hashed:parent-pin"},
	})
	hasher := userfakes.NewFakePasswordHasher()
	userRepo := userfakes.NewFakeUserRepository()
	_ = userRepo.Create(context.Background(), &domainuser.User{ID: "user-1", Username: "u1", PasswordHash: "hashed:secret"})
	mux := http.NewServeMux()
	registerParentalRoutes(mux, profileRepo, userRepo, hasher, gen)
	return mux, gen, "prof-kid"
}

// A kids profile must not be able to lift its OWN age cap. This is the same
// "forgotten twin" shape as the profile-creation escape: UpdateProfile over
// gRPC refuses parental changes made from a restricted profile
// (enforceParentalUpdateGate), but the hand-written HTTP route that mutates the
// very same fields had no equivalent gate - so one PUT neutralised the control
// on the whole catalogue, with no PIN and no profile switch.
func TestParentalUpdateSettings_FromKidsProfile_Refused(t *testing.T) {
	mux, gen, kidsID := newKidsCallerMux(t)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", ProfileID: kidsID, IsAdmin: false})

	req := httptest.NewRequest(http.MethodPut, "/api/v1/profiles/"+kidsID+"/parental", strings.NewReader(`{"maxAge":99}`))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("a kids profile must not raise its own max age: want 403, got %d body=%s", w.Code, w.Body.String())
	}
}

// The kids profile edits its SIBLING: the target is the parent profile, so the
// gate has to read the caller's own profile to know it is restricted. Every
// other test here edited the kids profile itself and never reached that read.
func TestParentalUpdateSettings_FromKidsProfile_OnSiblingRefused(t *testing.T) {
	mux, gen, kidsID := newKidsCallerMux(t)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", ProfileID: kidsID, IsAdmin: false})

	req := httptest.NewRequest(http.MethodPut, "/api/v1/profiles/prof-1/parental", strings.NewReader(`{"maxAge":99}`))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("a kids profile must not change a sibling's parental settings: want 403, got %d body=%s", w.Code, w.Body.String())
	}
}

// Clearing the blocked-tag list is the same escalation by another field.
func TestParentalUpdateSettings_FromKidsProfile_RefusesBlockedTagsWipe(t *testing.T) {
	mux, gen, kidsID := newKidsCallerMux(t)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", ProfileID: kidsID, IsAdmin: false})

	req := httptest.NewRequest(http.MethodPut, "/api/v1/profiles/"+kidsID+"/parental", strings.NewReader(`{"blockedTags":[]}`))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("a kids profile must not clear its own blocked tags: want 403, got %d body=%s", w.Code, w.Body.String())
	}
}

// A kids profile must not be able to turn its own protection off either - and
// that path is NOT covered by the PIN reauth gate, which only fires when a PIN
// already exists.
func TestParentalUpdateSettings_FromKidsProfile_RefusesDisable(t *testing.T) {
	mux, gen, kidsID := newKidsCallerMux(t)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", ProfileID: kidsID, IsAdmin: false})

	req := httptest.NewRequest(http.MethodPut, "/api/v1/profiles/"+kidsID+"/parental", strings.NewReader(`{"enabled":false}`))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("a kids profile must not disable its own protection: want 403, got %d body=%s", w.Code, w.Body.String())
	}
}

// The gate must not break the legitimate case: a parent on a NON-kids profile
// still manages the child's settings. Without this the fix would be a
// regression dressed as a security fix.
func TestParentalUpdateSettings_FromAdultProfile_StillAllowed(t *testing.T) {
	mux, gen, kidsID := newKidsCallerMux(t)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", ProfileID: "prof-1", IsAdmin: false})

	req := httptest.NewRequest(http.MethodPut, "/api/v1/profiles/"+kidsID+"/parental", strings.NewReader(`{"maxAge":12}`))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("a parent on a non-kids profile must still manage the child: want 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestParentalUpdateSettings_CrossProfileIDOR(t *testing.T) {
	mux, gen := newParentalMux(t)
	// user-1 tries to update parental on foreign profile
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", ProfileID: "prof-1", IsAdmin: false})
	body := `{"maxRating":"R","pin":"9999"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/profiles/prof-admin/parental", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d body=%s", w.Code, w.Body.String())
	}
}

// Every refusal this route emits is shown VERBATIM to the viewer: the fetcher
// hands `message` to the caller as the Error text, the web settings screen
// resolves it through t() only when it starts with "errors.", and the mobile
// parental screen puts it straight into an Alert. So a message that is not a
// catalogue key is English prose on a French phone - and one that IS a key but
// reaches a client that does not resolve it is a RAW KEY on screen, the exact
// beta.38 regression. The invariant that keeps both honest: this route only
// ever emits "errors.*" keys, and every client resolves them the same way.
func TestParentalUpdateSettings_RefusalsCarryTranslatableKeys(t *testing.T) {
	mux, gen, kidsID := newKidsCallerMux(t)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", ProfileID: kidsID, IsAdmin: false})

	req := httptest.NewRequest(http.MethodPut, "/api/v1/profiles/"+kidsID+"/parental", strings.NewReader(`{"maxAge":99}`))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("refusal body is not JSON: %v (%s)", err, w.Body.String())
	}
	if !strings.HasPrefix(body.Message, "errors.") {
		t.Fatalf("refusal must carry an i18n key, got English prose %q", body.Message)
	}
}

// The write side of a parental change: the kids/PIN household invariant is
// re-checked on the fully mutated profile (removing the guardian PIN while a
// kids profile exists is a 409 with a translatable key, nothing persisted),
// and a storage failure is a sanitized 500 rather than a false "ok".
func TestParentalUpdateSettings_PersistGuards(t *testing.T) {
	mux, gen, _ := newKidsCallerMux(t)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", ProfileID: "prof-1", IsAdmin: false})
	do := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/profiles/prof-1/parental", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w
	}

	// Clearing the parent's own PIN would leave the kids sibling unguarded.
	w := do(`{"currentPin":"parent-pin","pin":""}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("removing the guardian PIN next to a kids profile: want 409, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "errors.auth.pin_required_for_non_kids") {
		t.Errorf("409 must carry the translatable key, got %s", w.Body.String())
	}

	// A valid change on the parent profile persists.
	w = do(`{"currentPin":"parent-pin","blockedTags":["gore"]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("valid parental change: want 200, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Errorf("body = %s", w.Body.String())
	}
}
