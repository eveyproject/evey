package user

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
)

// --- config resolution ---

func TestResolveOIDCConfig_EnvOnlyDefaults(t *testing.T) {
	env := OIDCConfig{Enabled: true, Issuer: "https://idp", ClientID: "c", ClientSecret: "s"}
	cfg := ResolveOIDCConfig(env, "", nil, "https://evey.example/")
	if cfg.RedirectURL != "https://evey.example/api/v1/auth/oidc/callback" {
		t.Errorf("redirect = %q", cfg.RedirectURL)
	}
	if cfg.PostLogoutRedirectURL != "https://evey.example/login" {
		t.Errorf("post logout = %q", cfg.PostLogoutRedirectURL)
	}
	if !cfg.LinkByEmail {
		t.Error("linking by verified e-mail must default on")
	}
	if cfg.AllowSignup || cfg.AllowSignupDefault {
		t.Error("without a domain allowlist sign-up must default OFF (open sign-up with a public provider)")
	}
	if cfg.FromPanel {
		t.Error("no panel key: source must be the environment")
	}

	env.AllowedEmailDomains = []string{"example.com"}
	if cfg := ResolveOIDCConfig(env, "", nil, "https://e"); !cfg.AllowSignup {
		t.Error("behind a domain allowlist sign-up defaults on")
	}
	if cfg := ResolveOIDCConfig(env, "false", nil, "https://e"); cfg.AllowSignup {
		t.Error("EVEY_OIDC_ALLOW_SIGNUP=false must win over the default")
	}
	env.RedirectURL = "https://fixed/cb"
	if cfg := ResolveOIDCConfig(env, "", nil, "https://e"); cfg.RedirectURL != "https://fixed/cb" {
		t.Errorf("explicit EVEY_OIDC_REDIRECT_URL must be kept, got %q", cfg.RedirectURL)
	}
}

func TestResolveOIDCConfig_PanelWinsEvenWhenEmpty(t *testing.T) {
	env := OIDCConfig{Enabled: true, Issuer: "https://env-idp", ClientID: "env", ClientSecret: "env-secret", AllowedEmailDomains: []string{"env.com"}}
	panel := map[string]string{
		OIDCKeyEnabled:             "false",
		OIDCKeyIssuer:              " https://panel-idp ",
		OIDCKeyAllowedEmailDomains: "", // cleared in the panel: no allowlist
		OIDCKeyLinkByEmail:         "false",
		OIDCKeyDefaultLibraries:    "lib-a, lib-b",
		OIDCKeyButtonLabel:         "Sign in with Corp",
	}
	cfg := ResolveOIDCConfig(env, "true", panel, "https://e")
	if cfg.Enabled || cfg.Issuer != "https://panel-idp" || cfg.ClientID != "env" || cfg.ClientSecret != "env-secret" {
		t.Errorf("per-field precedence wrong: %+v", cfg)
	}
	if len(cfg.AllowedEmailDomains) != 0 {
		t.Errorf("an emptied panel list must override the env list, got %v", cfg.AllowedEmailDomains)
	}
	if cfg.LinkByEmail {
		t.Error("panel switch off must win")
	}
	if !cfg.AllowSignup {
		t.Error("env sign-up applies when the panel did not choose")
	}
	if len(cfg.DefaultLibraryIDs) != 2 || cfg.DefaultLibraryIDs[1] != "lib-b" || cfg.ButtonLabel != "Sign in with Corp" || !cfg.FromPanel {
		t.Errorf("panel values not applied: %+v", cfg)
	}
	panel[OIDCKeyAllowSignup] = "false"
	if ResolveOIDCConfig(env, "true", panel, "https://e").AllowSignup {
		t.Error("panel sign-up choice must win over the env")
	}
	panel[OIDCKeyDefaultLibraries] = OIDCDefaultLibrariesAll
	if ids := ResolveOIDCConfig(env, "", panel, "").DefaultLibraryIDs; ids != nil {
		t.Errorf("\"all\" must mean no restriction, got %v", ids)
	}
}

func TestParseEmailDomains(t *testing.T) {
	got := ParseEmailDomains(" Example.COM, @corp.org;;other.net\n")
	want := []string{"example.com", "corp.org", "other.net"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %v want %v", got, want)
		}
	}
}

func TestOIDCSettingsSource_ReadsStore(t *testing.T) {
	repo := sharedfakes.NewFakeServerConfigRepository()
	ctx := context.Background()
	_ = repo.Set(ctx, OIDCKeyIssuer, "https://panel")
	_ = repo.Set(ctx, "smtp.host", "ignored")
	src := OIDCSettingsSource{Env: OIDCConfig{Issuer: "https://env"}, Store: repo, BaseURL: func() string { return "https://b" }}
	cfg, err := src.Load(ctx)
	if err != nil || cfg.Issuer != "https://panel" || !cfg.FromPanel {
		t.Fatalf("cfg=%+v err=%v", cfg, err)
	}
	repo.Err = errors.New("db down")
	if _, err := src.Load(ctx); err == nil {
		t.Error("a store failure must surface, not silently fall back")
	}
}

// --- linking, sign-up and username policy ---

func newPolicySvc() (*OIDCService, *userfakes.FakeUserRepository, *userfakes.FakeLibraryAccessRepository) {
	repo := userfakes.NewFakeUserRepository()
	la := userfakes.NewFakeLibraryAccessRepository()
	return &OIDCService{
		userRepo:      repo,
		profiles:      userfakes.NewFakeProfileRepository(),
		events:        sharedfakes.NewFakeEventBus(),
		libraryAccess: la,
	}, repo, la
}

const testIssuer = "https://idp.test"

func localUser(t *testing.T, repo *userfakes.FakeUserRepository, name, email string) *domainuser.User {
	t.Helper()
	u, err := domainuser.NewUser(name, email, "hash")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	return u
}

func TestFindOrCreate_LinksLocalAccountOnVerifiedEmail(t *testing.T) {
	svc, repo, _ := newPolicySvc()
	local := localUser(t, repo, "bob", "bob@example.com")
	cfg := OIDCConfig{Issuer: testIssuer, LinkByEmail: true} // sign-up OFF: linking must not need it

	got, err := svc.findOrCreateUser(context.Background(), cfg, oidcIdentity{Sub: "s-bob", Email: "bob@example.com", EmailVerified: true, PreferredUsername: "bobby"})
	if err != nil {
		t.Fatalf("link: %v", err)
	}
	if got.ID != local.ID {
		t.Fatalf("signed into %s, want the existing account %s", got.ID, local.ID)
	}
	linked, err := repo.GetByOIDCSubject(context.Background(), testIssuer, "s-bob")
	if err != nil || linked.ID != local.ID {
		t.Fatalf("identity not persisted on the local account: %v", err)
	}
	if linked.PasswordHash != "hash" {
		t.Error("linking must not touch the password")
	}
	// Second login takes the (issuer, subject) path.
	again, err := svc.findOrCreateUser(context.Background(), cfg, oidcIdentity{Sub: "s-bob", Email: "changed@example.com"})
	if err != nil || again.ID != local.ID {
		t.Fatalf("second login: %v", err)
	}
}

func TestFindOrCreate_NeverLinksOnUnverifiedEmail(t *testing.T) {
	svc, repo, _ := newPolicySvc()
	localUser(t, repo, "bob", "bob@example.com")
	cfg := OIDCConfig{Issuer: testIssuer, LinkByEmail: true, AllowSignup: true}

	_, err := svc.findOrCreateUser(context.Background(), cfg, oidcIdentity{Sub: "s-evil", Email: "bob@example.com", EmailVerified: false})
	if !errors.Is(err, ErrOIDCAccountExists) {
		t.Fatalf("unverified e-mail matching a local account: got %v, want ErrOIDCAccountExists", err)
	}
	if _, err := repo.GetByOIDCSubject(context.Background(), testIssuer, "s-evil"); !errors.Is(err, shared.ErrNotFound) {
		t.Error("an unverified e-mail linked the account (account takeover)")
	}
}

func TestFindOrCreate_NeverLinksAnAdminByEmail(t *testing.T) {
	svc, repo, _ := newPolicySvc()
	u := localUser(t, repo, "root", "root@example.com")
	u.IsAdmin = true
	_ = repo.Update(context.Background(), u)
	cfg := OIDCConfig{Issuer: testIssuer, LinkByEmail: true, AllowSignup: true}
	_, err := svc.findOrCreateUser(context.Background(), cfg, oidcIdentity{Sub: "s-root", Email: "root@example.com", EmailVerified: true})
	if !errors.Is(err, ErrOIDCAccountExists) {
		t.Fatalf("got %v, want ErrOIDCAccountExists", err)
	}
	if _, err := repo.GetByOIDCSubject(context.Background(), testIssuer, "s-root"); !errors.Is(err, shared.ErrNotFound) {
		t.Error("an administrator account was linked by e-mail")
	}
}

func TestFindOrCreate_LinkSwitchOff(t *testing.T) {
	svc, repo, _ := newPolicySvc()
	localUser(t, repo, "bob", "bob@example.com")
	cfg := OIDCConfig{Issuer: testIssuer, LinkByEmail: false, AllowSignup: true}
	_, err := svc.findOrCreateUser(context.Background(), cfg, oidcIdentity{Sub: "s-bob", Email: "bob@example.com", EmailVerified: true})
	if !errors.Is(err, ErrOIDCAccountExists) {
		t.Fatalf("got %v, want ErrOIDCAccountExists", err)
	}
}

func TestFindOrCreate_DisabledLocalAccountStaysRefused(t *testing.T) {
	svc, repo, _ := newPolicySvc()
	u := localUser(t, repo, "off", "off@example.com")
	u.Enabled = false
	_ = repo.Update(context.Background(), u)
	cfg := OIDCConfig{Issuer: testIssuer, LinkByEmail: true}
	_, err := svc.findOrCreateUser(context.Background(), cfg, oidcIdentity{Sub: "s-off", Email: "off@example.com", EmailVerified: true})
	if !errors.Is(err, ErrUserDisabled) {
		t.Fatalf("got %v, want ErrUserDisabled", err)
	}
	if _, err := repo.GetByOIDCSubject(context.Background(), testIssuer, "s-off"); err == nil {
		t.Error("a disabled account was linked")
	}
}

func TestFindOrCreate_AccountLinkedToAnotherIdentity(t *testing.T) {
	svc, repo, _ := newPolicySvc()
	u := localUser(t, repo, "bob", "bob@example.com")
	if err := repo.LinkOIDCIdentity(context.Background(), u.ID, testIssuer, "s-first"); err != nil {
		t.Fatal(err)
	}
	cfg := OIDCConfig{Issuer: testIssuer, LinkByEmail: true, AllowSignup: true}
	_, err := svc.findOrCreateUser(context.Background(), cfg, oidcIdentity{Sub: "s-second", Email: "bob@example.com", EmailVerified: true})
	if !errors.Is(err, ErrOIDCAccountExists) {
		t.Fatalf("got %v, want ErrOIDCAccountExists", err)
	}
}

func TestFindOrCreate_SignupDisabledRefusesUnknown(t *testing.T) {
	svc, repo, _ := newPolicySvc()
	cfg := OIDCConfig{Issuer: testIssuer, LinkByEmail: true, AllowSignup: false}
	_, err := svc.findOrCreateUser(context.Background(), cfg, oidcIdentity{Sub: "s-new", Email: "new@example.com", EmailVerified: true, PreferredUsername: "new"})
	if !errors.Is(err, ErrOIDCSignupDisabled) {
		t.Fatalf("got %v, want ErrOIDCSignupDisabled", err)
	}
	if all, _ := repo.List(context.Background()); len(all) != 0 {
		t.Errorf("an account was created with sign-up off: %d", len(all))
	}
}

func TestFindOrCreate_DomainAllowlistAppliesToLinkingToo(t *testing.T) {
	svc, repo, _ := newPolicySvc()
	localUser(t, repo, "bob", "bob@other.org")
	cfg := OIDCConfig{Issuer: testIssuer, LinkByEmail: true, AllowSignup: true, AllowedEmailDomains: []string{"example.com"}}
	_, err := svc.findOrCreateUser(context.Background(), cfg, oidcIdentity{Sub: "s", Email: "bob@other.org", EmailVerified: true})
	if !errors.Is(err, ErrEmailDomainNotAllowed) {
		t.Fatalf("got %v, want ErrEmailDomainNotAllowed", err)
	}
}

func TestFindOrCreate_UsernameCollisionGetsDeterministicSuffix(t *testing.T) {
	svc, repo, _ := newPolicySvc()
	localUser(t, repo, "Alice", "alice@home.lan")
	cfg := OIDCConfig{Issuer: testIssuer, AllowSignup: true}
	ident := oidcIdentity{Sub: "s-alice", Email: "alice@corp.example", EmailVerified: true, PreferredUsername: "alice"}

	got, err := svc.findOrCreateUser(context.Background(), cfg, ident)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	want := oidcUsernameCandidates(testIssuer, "s-alice", "alice", "")[1]
	if got.Username != want || got.Username == "alice" {
		t.Fatalf("username = %q, want the deterministic %q", got.Username, want)
	}
	if again := oidcUsernameCandidates(testIssuer, "s-alice", "alice", ""); again[1] != want {
		t.Error("the suffix must be stable for the same identity")
	}
	if other := oidcUsernameCandidates(testIssuer, "s-other", "alice", ""); other[1] == want {
		t.Error("two identities must not share a suffix")
	}
}

func TestFindOrCreate_DefaultLibraryAccess(t *testing.T) {
	svc, repo, la := newPolicySvc()
	cfg := OIDCConfig{Issuer: testIssuer, AllowSignup: true, DefaultLibraryIDs: []shared.LibraryID{"lib-movies"}}
	u, err := svc.findOrCreateUser(context.Background(), cfg, oidcIdentity{Sub: "s-kid", Email: "kid@example.com", EmailVerified: true, PreferredUsername: "kid"})
	if err != nil {
		t.Fatal(err)
	}
	grants, _ := la.GetByUserID(context.Background(), u.ID)
	if len(grants) != 1 || grants[0] != "lib-movies" {
		t.Fatalf("grants = %v, want [lib-movies]", grants)
	}

	// "All libraries" writes no grant (zero grants = everything).
	u2, err := svc.findOrCreateUser(context.Background(), OIDCConfig{Issuer: testIssuer, AllowSignup: true}, oidcIdentity{Sub: "s-all", PreferredUsername: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if g, _ := la.GetByUserID(context.Background(), u2.ID); len(g) != 0 {
		t.Errorf("all-libraries account got grants %v", g)
	}
	_ = repo
}

// GrantMany silently skips deleted libraries; a selection that vanished must
// refuse the sign-up rather than hand out access to everything.
type emptyGrantRepo struct {
	*userfakes.FakeLibraryAccessRepository
}

func (emptyGrantRepo) GrantMany(context.Context, shared.UserID, []shared.LibraryID) error { return nil }

func TestFindOrCreate_DeletedDefaultLibrariesFailClosed(t *testing.T) {
	svc, _, _ := newPolicySvc()
	svc.libraryAccess = emptyGrantRepo{userfakes.NewFakeLibraryAccessRepository()}
	svc.uow = sharedfakes.NewFakeTransactionalUnit()
	cfg := OIDCConfig{Issuer: testIssuer, AllowSignup: true, DefaultLibraryIDs: []shared.LibraryID{"gone"}}
	if _, err := svc.findOrCreateUser(context.Background(), cfg, oidcIdentity{Sub: "s", PreferredUsername: "x"}); err == nil {
		t.Fatal("sign-up succeeded with zero grants, i.e. access to every library")
	}
}

// --- lazy discovery ---

func TestOIDCService_ProviderDownNeverBlocksAndRecovers(t *testing.T) {
	var hits atomic.Int32
	up := atomic.Bool{}
	oidcSrv := newTestOIDCServer(t)
	defer oidcSrv.close()
	inner := oidcSrv.srv.Config.Handler
	oidcSrv.srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			hits.Add(1)
			if !up.Load() {
				http.Error(w, "down", http.StatusServiceUnavailable)
				return
			}
		}
		inner.ServeHTTP(w, r)
	})

	cfg := OIDCConfig{Enabled: true, Issuer: oidcSrv.issuer, ClientID: "c", ClientSecret: "s", RedirectURL: "http://x/cb"}
	svc, err := NewOIDCService(func(context.Context) (OIDCConfig, error) { return cfg, nil },
		userfakes.NewFakeUserRepository(), userfakes.NewFakeProfileRepository(), userfakes.NewFakeRefreshTokenRepository(),
		userfakes.NewFakeUserRoleRepository(userfakes.NewFakeRoleRepository()), userfakes.NewFakeTokenGenerator(),
		sharedfakes.NewFakeEventBus(), nil, 0)
	if err != nil {
		t.Fatalf("construction must not contact the provider: %v", err)
	}
	if hits.Load() != 0 {
		t.Fatal("discovery ran at construction")
	}
	ctx := context.Background()
	if ok, _ := svc.Available(ctx); ok {
		t.Fatal("SSO offered while the provider is down")
	}
	if _, st := svc.Status(ctx, false); st.State != "error" || st.Error == "" {
		t.Fatalf("status = %+v, want error with a reason", st)
	}
	if _, err := svc.AuthURL(ctx, "n"); !errors.Is(err, ErrOIDCUnavailable) {
		t.Fatalf("AuthURL err = %v, want ErrOIDCUnavailable", err)
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("discovery hits = %d during the backoff, want 1", n)
	}

	up.Store(true)
	if _, st := svc.Status(ctx, true); st.State != "ok" || st.EndSessionEndpoint == "" {
		t.Fatalf("after recovery status = %+v", st)
	}
	if ok, _ := svc.Available(ctx); !ok {
		t.Fatal("SSO not offered after the provider came back")
	}
	before := hits.Load()
	cfg.ButtonLabel = "Corp" // policy-only change keeps the provider
	if ok, label := svc.Available(ctx); !ok || label != "Corp" || hits.Load() != before {
		t.Errorf("policy change: ok=%v label=%q rediscovered=%v", ok, label, hits.Load() != before)
	}
}

func TestOIDCService_DisabledAndIncomplete(t *testing.T) {
	cfg := OIDCConfig{Enabled: false}
	svc, _ := NewOIDCService(func(context.Context) (OIDCConfig, error) { return cfg, nil }, nil, nil, nil, nil, nil, nil, nil, 0)
	if _, st := svc.Status(context.Background(), false); st.State != "disabled" {
		t.Errorf("state = %q", st.State)
	}
	cfg = OIDCConfig{Enabled: true, Issuer: "https://idp", RedirectURL: "x"}
	if _, st := svc.Status(context.Background(), false); st.State != "incomplete" || st.Error != "client_id is not set" {
		t.Errorf("status = %+v", st)
	}
}

func TestOIDCService_ProbeDoesNotTouchCache(t *testing.T) {
	oidcSrv := newTestOIDCServer(t)
	defer oidcSrv.close()
	svc, _ := NewOIDCService(func(context.Context) (OIDCConfig, error) { return OIDCConfig{}, nil }, nil, nil, nil, nil, nil, nil, nil, 0)
	ep, err := svc.Probe(context.Background(), oidcSrv.issuer)
	if err != nil || ep != oidcSrv.issuer+"/logout" {
		t.Fatalf("probe ep=%q err=%v", ep, err)
	}
	bad := httptest.NewServer(http.NotFoundHandler())
	defer bad.Close()
	if _, err := svc.Probe(context.Background(), bad.URL); err == nil {
		t.Error("probe of a non-provider must fail")
	}
	if svc.cur != nil {
		t.Error("probe stored a client")
	}
}
