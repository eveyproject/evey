package demo_test

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appdemo "github.com/evey-project/evey/internal/app/demo"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/activity"
	"github.com/evey-project/evey/internal/domain/feed"
	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	"github.com/evey-project/evey/internal/domain/playback"
	pbfakes "github.com/evey-project/evey/internal/domain/playback/fakes"
	"github.com/evey-project/evey/internal/domain/plugin"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/shared/fakes"
	"github.com/evey-project/evey/internal/domain/user"
	ufakes "github.com/evey-project/evey/internal/domain/user/fakes"
	"github.com/evey-project/evey/internal/pkg/demo"
)

// repoLibraries stands in for the library service: creates through the repo,
// records the scans it was asked for.
type repoLibraries struct {
	repo  *libfakes.FakeLibraryRepository
	scans []string
}

func (l *repoLibraries) CreateLibrary(ctx context.Context, lib *library.Library) error {
	return l.repo.Create(ctx, lib)
}

func (l *repoLibraries) UpdateLibrary(ctx context.Context, lib *library.Library) error {
	return l.repo.Update(ctx, lib)
}

func (l *repoLibraries) ScanLibrary(_ context.Context, id string) (int, int, error) {
	l.scans = append(l.scans, id)
	return 0, 0, nil
}

func (l *repoLibraries) DeleteLibrary(ctx context.Context, id string) error {
	return l.repo.Delete(ctx, shared.LibraryID(id))
}

type harness struct {
	users    *ufakes.FakeUserRepository
	profiles *ufakes.FakeProfileRepository
	libs     *repoLibraries
	access   *ufakes.FakeLibraryAccessRepository
	marker   *fakes.FakeServerConfigRepository
	boot     *appdemo.Bootstrapper
	dir      string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	users := ufakes.NewFakeUserRepository()
	profiles := ufakes.NewFakeProfileRepository()
	refresh := ufakes.NewFakeRefreshTokenRepository()
	roles := ufakes.NewFakeRoleRepository()
	hasher := ufakes.NewFakePasswordHasher()
	bus := fakes.NewFakeEventBus()
	setup := appuser.NewCommandService(users, profiles, refresh, roles, ufakes.NewFakeUserRoleRepository(roles),
		hasher, ufakes.NewFakeTokenGenerator(), bus, 7*24*time.Hour, ufakes.NewFakeQuickConnectRepository(),
		appuser.WithMinPasswordLength(demo.MinPasswordLength))
	accounts := appuser.NewUserService(users, profiles, refresh, hasher, bus, fakes.NewFakeTransactionalUnit(),
		appuser.UserWithMinPasswordLength(demo.MinPasswordLength))
	libRepo := libfakes.NewFakeLibraryRepository()
	libs := &repoLibraries{repo: libRepo}
	access := ufakes.NewFakeLibraryAccessRepository()
	dir := t.TempDir()
	marker := fakes.NewFakeServerConfigRepository()
	boot := appdemo.NewBootstrapper(users, profiles, setup, accounts, libRepo, libs, hasher, dir, slog.Default()).
		WithLibraryAccess(access).WithMarker(marker)
	return &harness{users: users, profiles: profiles, libs: libs, access: access, marker: marker, boot: boot, dir: dir}
}

// A fresh instance: the demo administrator comes through the wizard's Setup
// with the demo password floor, the four accounts and their profiles exist
// with the rules they illustrate, the four libraries point at the media
// folders. A second run changes nothing.
func TestBootstrap_FreshInstanceIsIdempotent(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	if err := h.boot.EnsureAdmin(ctx); err != nil {
		t.Fatalf("EnsureAdmin: %v", err)
	}
	admin, gerr := h.users.GetByUsername(ctx, demo.Username)
	if gerr != nil || admin == nil || !admin.IsAdmin {
		t.Fatalf("demo admin not created as administrator: %v %+v", gerr, admin)
	}
	if admin.PasswordHash != "hashed:"+demo.Password {
		t.Fatalf("demo admin password hash = %q, want the published password", admin.PasswordHash)
	}

	if err := h.boot.SeedAccounts(ctx); err != nil {
		t.Fatalf("SeedAccounts: %v", err)
	}
	fam, err := h.users.GetByUsername(ctx, "famille")
	if err != nil || fam == nil {
		t.Fatalf("famille missing: %v", err)
	}
	if fam.MaxStreams == nil || *fam.MaxStreams != 2 || fam.IsAdmin {
		t.Errorf("famille limits = %+v, want 2 streams and not admin", fam)
	}
	profs, _ := h.profiles.GetByUserID(ctx, fam.ID)
	if len(profs) != 2 {
		t.Fatalf("famille profiles = %d, want 2 (Camille + Lou)", len(profs))
	}
	var kids, adultWithPIN int
	for _, p := range profs {
		if p.IsKids && p.Parental != nil && p.Parental.MaxAge != nil && *p.Parental.MaxAge == 10 {
			kids++
		}
		if !p.IsKids && p.Parental != nil && p.Parental.PINHash != "" && p.IsDefault {
			adultWithPIN++
		}
	}
	if kids != 1 || adultWithPIN != 1 {
		t.Errorf("household: kids=%d adultWithPIN=%d, want 1 and 1 (the kids rule needs the adult PIN)", kids, adultWithPIN)
	}
	if old, _ := h.users.GetByUsername(ctx, "ancien"); old == nil || old.Enabled {
		t.Errorf("ancien must exist and be disabled")
	}

	libs, created, err := h.boot.EnsureLibraries(ctx)
	if err != nil {
		t.Fatalf("EnsureLibraries: %v", err)
	}
	if len(libs) != 4 || len(created) != 4 {
		t.Fatalf("libraries = %d created = %d, want 4 and 4", len(libs), len(created))
	}
	all, _ := h.libs.repo.List(ctx, library.ListLibrariesFilter{})
	if len(all) != 4 {
		t.Fatalf("repo libraries = %d, want 4", len(all))
	}
	for _, l := range all {
		if len(l.Paths) != 1 || filepath.Dir(l.Paths[0]) != h.dir {
			t.Errorf("library %s paths = %v, want one folder under %s", l.Name, l.Paths, h.dir)
		}
		if st, err := os.Stat(l.Paths[0]); err != nil || !st.IsDir() {
			t.Errorf("library folder %s not created", l.Paths[0])
		}
	}
	if err := h.boot.ConfineAccounts(ctx, libs); err != nil {
		t.Fatalf("ConfineAccounts: %v", err)
	}
	guest, _ := h.users.GetByUsername(ctx, "invite")
	got, _ := h.access.GetByUserID(ctx, guest.ID)
	if len(got) != 1 || got[0] != libs["movies"] {
		t.Errorf("invite access = %v, want the films library only", got)
	}

	// Second run: nothing duplicated.
	for _, step := range []func() error{
		func() error { return h.boot.EnsureAdmin(ctx) },
		func() error { return h.boot.SeedAccounts(ctx) },
		func() error {
			_, again, err := h.boot.EnsureLibraries(ctx)
			if len(again) != 0 {
				t.Errorf("second run created %v", again)
			}
			return err
		},
	} {
		if err := step(); err != nil {
			t.Fatalf("second run: %v", err)
		}
	}
	if n, _ := h.users.CountAdmins(ctx); n != 1 {
		t.Errorf("admins after second run = %d, want 1", n)
	}
	all, _ = h.libs.repo.List(ctx, library.ListLibrariesFilter{})
	if len(all) != 4 {
		t.Errorf("libraries after second run = %d, want 4", len(all))
	}
}

// An instance that already has an administrator (a dev box): the demo
// administrator is added beside it, and a disabled one is restored.
func TestBootstrap_EnsureAdminBesideAnExistingAdministrator(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	other, _ := user.NewUser("owner", "owner@example.com", "hashed:x")
	other.IsAdmin = true
	if err := h.users.Create(ctx, other); err != nil {
		t.Fatal(err)
	}
	if err := h.boot.EnsureAdmin(ctx); err != nil {
		t.Fatalf("EnsureAdmin: %v", err)
	}
	d, _ := h.users.GetByUsername(ctx, demo.Username)
	if d == nil || !d.IsAdmin || !d.Enabled {
		t.Fatalf("demo admin beside owner = %+v", d)
	}
	d.Enabled = false
	_ = h.users.Update(ctx, d)
	if err := h.boot.EnsureAdmin(ctx); err != nil {
		t.Fatal(err)
	}
	if d2, _ := h.users.GetByUsername(ctx, demo.Username); !d2.Enabled {
		t.Error("a disabled demo admin must be re-enabled")
	}
}

// Downloads land through a .part file, a zip member is extracted, an existing
// file is not fetched again, and the library whose folder received a file is
// scanned once.
func TestBootstrap_DownloadMediaFetchesMissingFilesAndScans(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	w, _ := zw.Create("inner.mp4")
	_, _ = w.Write([]byte("zipped-video"))
	_ = zw.Close()
	hits := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		hits[r.URL.Path]++
		switch r.URL.Path {
		case "/plain.jpg":
			_, _ = rw.Write([]byte("jpeg-bytes"))
		case "/master.zip":
			_, _ = rw.Write(zipBuf.Bytes())
		default:
			rw.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	manifest := []demo.MediaFile{
		{URL: srv.URL + "/plain.jpg", Path: "photos/Demo 01.jpg"},
		{URL: srv.URL + "/missing.mp4", Path: "movies/Gone (2000).mp4"},
		{URL: srv.URL + "/master.zip", Path: "movies/Big (2008).mp4", ZipMember: "inner.mp4"},
	}
	// The photo already exists: it must not be fetched.
	_ = os.MkdirAll(filepath.Join(h.dir, "photos"), 0o755)
	_ = os.WriteFile(filepath.Join(h.dir, "photos", "Demo 01.jpg"), []byte("already"), 0o644)

	libs := map[string]shared.LibraryID{"photos": "lib-p", "movies": "lib-m"}
	if err := h.boot.WithHTTPClient(srv.Client()).DownloadManifest(ctx, manifest, libs); err != nil {
		t.Fatalf("DownloadManifest: %v", err)
	}
	if hits["/plain.jpg"] != 0 {
		t.Error("an existing file was downloaded again")
	}
	if b, err := os.ReadFile(filepath.Join(h.dir, "movies", "Big (2008).mp4")); err != nil || string(b) != "zipped-video" {
		t.Errorf("zip member not extracted: %v %q", err, b)
	}
	if _, err := os.Stat(filepath.Join(h.dir, "movies", "Gone (2000).mp4")); err == nil {
		t.Error("a failed download must leave no file")
	}
	entries, _ := os.ReadDir(filepath.Join(h.dir, "movies"))
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".part" || filepath.Ext(e.Name()) == ".unzip" || filepath.Ext(e.Name()) == ".zip" {
			t.Errorf("temporary file left behind: %s", e.Name())
		}
	}
	if len(h.libs.scans) != 1 || h.libs.scans[0] != "lib-m" {
		t.Errorf("scans = %v, want the films library once (the photo folder received nothing)", h.libs.scans)
	}
}

// The embedded manifest itself: every path stays inside one of the four
// library folders, no two entries collide, every URL is https.
func TestManifest_PathsAreConfinedAndUnique(t *testing.T) {
	folders := map[string]bool{}
	for _, l := range demo.Libraries {
		folders[l.Folder] = true
	}
	seen := map[string]bool{}
	for _, f := range demo.Manifest {
		clean := filepath.ToSlash(filepath.Clean(f.Path))
		if clean != f.Path || filepath.IsAbs(f.Path) {
			t.Errorf("path %q is not a clean relative path", f.Path)
		}
		top := clean
		if i := bytes.IndexByte([]byte(clean), '/'); i > 0 {
			top = clean[:i]
		}
		if !folders[top] {
			t.Errorf("path %q is outside the library folders", f.Path)
		}
		if seen[clean] {
			t.Errorf("path %q listed twice", f.Path)
		}
		seen[clean] = true
		if len(f.URL) < 8 || f.URL[:8] != "https://" {
			t.Errorf("url %q is not https", f.URL)
		}
	}
	if len(demo.Manifest) < 20 {
		t.Errorf("manifest has %d entries, the demo catalogue should not shrink below 20", len(demo.Manifest))
	}
}

// Boot without the flag after a boot with it: the demo libraries, the visitor
// accounts, the manifest files and the marker are gone; the operator's own
// library and account are untouched; the only administrator left is kept but
// no longer opens with the published password. An instance that never ran the
// demo reports no marker, so nothing runs there.
func TestTeardown_RemovesWhatTheBootstrapCreatedAndNothingElse(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	// The operator's own things, created before the demo ran.
	own, _ := user.NewUser("owner", "owner@example.com", "hashed:x")
	own.IsAdmin = true
	_ = h.users.Create(ctx, own)
	ownLib, _ := library.NewLibrary("lib-own", "Mes films", library.Movie, library.WithLibraryPaths([]string{"/media/own"}))
	_ = h.libs.repo.Create(ctx, ownLib)
	if h.boot.WasBootstrapped(ctx) {
		t.Fatal("a fresh instance must not report a bootstrap")
	}

	if err := h.boot.EnsureAdmin(ctx); err != nil {
		t.Fatal(err)
	}
	if err := h.boot.SeedAccounts(ctx); err != nil {
		t.Fatal(err)
	}
	libs, _, err := h.boot.EnsureLibraries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.boot.MarkBootstrapped(ctx); err != nil {
		t.Fatal(err)
	}
	if !h.boot.WasBootstrapped(ctx) {
		t.Fatal("marker not recorded")
	}
	// A manifest file "downloaded", a stray operator file next to it.
	first := filepath.Join(h.dir, filepath.FromSlash(demo.Manifest[0].Path))
	_ = os.MkdirAll(filepath.Dir(first), 0o755)
	_ = os.WriteFile(first, []byte("x"), 0o644)
	stray := filepath.Join(h.dir, "movies", "mine.mkv")
	_ = os.WriteFile(stray, []byte("mine"), 0o644)
	_ = libs

	if err := h.boot.Teardown(ctx); err != nil {
		t.Fatalf("Teardown: %v", err)
	}
	all, _ := h.libs.repo.List(ctx, library.ListLibrariesFilter{})
	if len(all) != 1 || all[0].Name != "Mes films" {
		t.Errorf("libraries after teardown = %v, want the operator's only", all)
	}
	for _, name := range []string{"famille", "solo", "invite", "ancien", demo.Username} {
		if u, _ := h.users.GetByUsername(ctx, name); u != nil {
			t.Errorf("%s still exists after teardown", name)
		}
	}
	if u, _ := h.users.GetByUsername(ctx, "owner"); u == nil {
		t.Error("the operator's account was removed")
	}
	if _, err := os.Stat(first); err == nil {
		t.Error("manifest file not removed")
	}
	if _, err := os.Stat(stray); err != nil {
		t.Error("a file the bootstrap did not create was removed")
	}
	if h.boot.WasBootstrapped(ctx) {
		t.Error("marker still present after teardown")
	}
}

// When `demo` is the only administrator, deleting it would reopen the setup
// wizard on a live server: it is kept, its password is no longer the
// published one, and the operator is told how to recover.
func TestTeardown_LastAdministratorKeepsAccountWithRotatedPassword(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if err := h.boot.EnsureAdmin(ctx); err != nil {
		t.Fatal(err)
	}
	if err := h.boot.Teardown(ctx); err != nil {
		t.Fatalf("Teardown: %v", err)
	}
	d, _ := h.users.GetByUsername(ctx, demo.Username)
	if d == nil {
		t.Fatal("the last administrator must not be deleted")
	}
	if d.PasswordHash == "hashed:"+demo.Password {
		t.Error("the published password must not survive the teardown")
	}
}

// fakePlugins stands in for the plugin repository and both plugin services,
// recording every call so the test can read what the bootstrap asked for.
type fakePlugins struct {
	byName      map[string]*plugin.Plugin
	installed   []string
	uninstalled []string
	enabled     []string
	configs     map[string]map[string]any
}

func newFakePlugins() *fakePlugins {
	return &fakePlugins{byName: map[string]*plugin.Plugin{}, configs: map[string]map[string]any{}}
}

func (f *fakePlugins) Create(_ context.Context, p *plugin.Plugin) error {
	f.byName[p.Name] = p
	return nil
}
func (f *fakePlugins) GetByID(_ context.Context, id string) (*plugin.Plugin, error) {
	for _, p := range f.byName {
		if p.ID == id {
			return p, nil
		}
	}
	return nil, shared.ErrNotFound
}
func (f *fakePlugins) GetByName(_ context.Context, name string) (*plugin.Plugin, error) {
	if p, ok := f.byName[name]; ok {
		return p, nil
	}
	return nil, shared.ErrNotFound
}
func (f *fakePlugins) List(context.Context) ([]*plugin.Plugin, error) { return nil, nil }
func (f *fakePlugins) Update(context.Context, *plugin.Plugin) error   { return nil }
func (f *fakePlugins) UpdateConfig(context.Context, string, map[string]any) error {
	return nil
}
func (f *fakePlugins) Delete(context.Context, string) error { return nil }

func (f *fakePlugins) InstallPlugin(_ context.Context, source string) (*plugin.Plugin, error) {
	f.installed = append(f.installed, source)
	// The real install starts the plugin and returns the record from BEFORE
	// that: the stored row is running, the returned copy is not.
	name := strings.TrimSuffix(filepath.Base(source), ".zip")
	p := &plugin.Plugin{ID: "id-" + name, Name: name, Version: "2.18.1", Enabled: true, Status: plugin.Running}
	f.byName[p.Name] = p
	p.Status = plugin.Initializing // the handshake has not landed yet
	stale := *p
	stale.Enabled, stale.Status = false, plugin.Installed
	return &stale, nil
}
func (f *fakePlugins) UninstallPlugin(_ context.Context, id string) error {
	f.uninstalled = append(f.uninstalled, id)
	for name, p := range f.byName {
		if p.ID == id {
			delete(f.byName, name)
		}
	}
	return nil
}
func (f *fakePlugins) EnablePlugin(_ context.Context, id string) error {
	f.enabled = append(f.enabled, id)
	return nil
}
func (f *fakePlugins) UpdatePluginConfig(_ context.Context, id string, cfg map[string]any) error {
	f.configs[id] = cfg
	return nil
}

// The weather-v2 plugin is installed from the bundle under the plugin dir,
// configured on Paris and enabled; the retired v1 seed is uninstalled; a
// second boot re-applies the config and installs nothing; a missing bundle is
// skipped, not an error; the teardown uninstalls by name and leaves the
// operator's plugins alone.
func TestPlugins_InstalledFromBundleConfiguredAndRemoved(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	fp := newFakePlugins()
	own := &plugin.Plugin{ID: "own-1", Name: "own-theme"}
	_ = fp.Create(ctx, own)
	_ = fp.Create(ctx, &plugin.Plugin{ID: "old-weather", Name: "weather"})
	pluginDir := t.TempDir()
	h.boot.WithPlugins(fp, fp, fp, pluginDir)

	// Bundle absent: skipped, no id; the retired seed still goes.
	ids, err := h.boot.EnsurePlugins(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(fp.installed) != 0 || len(ids) != 0 {
		t.Fatalf("installed without a bundle: %v %v", fp.installed, ids)
	}
	if len(fp.uninstalled) != 1 || fp.uninstalled[0] != "old-weather" {
		t.Fatalf("retired plugin not removed: %v", fp.uninstalled)
	}

	bundle := filepath.Join(pluginDir, "weather-v2.zip")
	_ = os.WriteFile(bundle, []byte("zip"), 0o644)
	ids, err = h.boot.EnsurePlugins(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(fp.installed) != 1 || fp.installed[0] != bundle {
		t.Fatalf("installed = %v, want the bundle once", fp.installed)
	}
	id := fp.byName["weather-v2"].ID
	if ids["weather-v2"] != id {
		t.Errorf("ids = %v, want weather-v2 -> %s", ids, id)
	}
	if got := fp.configs[id]["city"]; got != "Paris" {
		t.Errorf("city = %v, want Paris", got)
	}
	if len(fp.enabled) != 0 {
		t.Errorf("enabled = %v, want none: the install already started it", fp.enabled)
	}

	// Second boot with the plugin disabled (a visitor cannot, an operator on
	// a dev box can): re-enabled, config re-applied, nothing reinstalled.
	fp.byName["weather-v2"].Enabled, fp.byName["weather-v2"].Status = false, plugin.Installed
	fp.configs[id] = map[string]any{"city": "Lyon"}
	if _, err := h.boot.EnsurePlugins(ctx); err != nil {
		t.Fatal(err)
	}
	if len(fp.installed) != 1 || len(fp.enabled) != 1 || fp.enabled[0] != id {
		t.Errorf("second boot: installed=%v enabled=%v, want one install, one enable", fp.installed, fp.enabled)
	}
	if got := fp.configs[id]["city"]; got != "Paris" {
		t.Errorf("config not re-applied: city = %v", got)
	}

	if err := h.boot.RemovePlugins(ctx); err != nil {
		t.Fatal(err)
	}
	if len(fp.uninstalled) != 2 || fp.uninstalled[1] != id {
		t.Errorf("uninstalled = %v, want the retired one then %s", fp.uninstalled, id)
	}
	if _, err := fp.GetByName(ctx, "own-theme"); err != nil {
		t.Error("the operator's plugin was removed")
	}
}

type fakeHome struct{ cfgs []*feed.AdminConfig }

func (f *fakeHome) BatchUpdateAdminConfigs(_ context.Context, cfgs []*feed.AdminConfig) error {
	f.cfgs = cfgs
	return nil
}

// The home is written in the manifest's order - weather, resume, the three
// libraries, most popular - with every other built-in section and the photo
// library disabled behind them. A plugin without a bundle drops out of the
// order rather than leaving a hole.
func TestArrangeHome_WritesTheManifestOrderAndHidesTheRest(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if err := h.boot.EnsureAdmin(ctx); err != nil {
		t.Fatal(err)
	}
	libs, _, err := h.boot.EnsureLibraries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	home := &fakeHome{}
	h.boot.WithHomeLayout(home)

	if err := h.boot.ArrangeHome(ctx, libs, map[string]string{"weather-v2": "plg-1"}); err != nil {
		t.Fatal(err)
	}
	var shown []string
	hidden := map[string]bool{}
	for i, c := range home.cfgs {
		if c.Enabled {
			if c.DisplayOrder != i {
				t.Errorf("%s at order %d, want %d", c.FeedType, c.DisplayOrder, i)
			}
			shown = append(shown, string(c.FeedType))
		} else {
			hidden[string(c.FeedType)] = true
		}
	}
	want := []string{"plugin:plg-1", "resume_playback", "library:" + string(libs["movies"]), "library:" + string(libs["series"]),
		"library:" + string(libs["music"]), "most_popular"}
	if strings.Join(shown, ",") != strings.Join(want, ",") {
		t.Errorf("shown = %v, want %v", shown, want)
	}
	for _, ft := range []string{"recently_added", "next_episode", "collections", "library:" + string(libs["photos"])} {
		if !hidden[ft] {
			t.Errorf("%s should be disabled", ft)
		}
	}

	// No plugin installed: the order simply starts at resume.
	if err := h.boot.ArrangeHome(ctx, libs, nil); err != nil {
		t.Fatal(err)
	}
	if home.cfgs[0].FeedType != "resume_playback" || home.cfgs[0].DisplayOrder != 0 {
		t.Errorf("without the plugin the first section is %s at %d", home.cfgs[0].FeedType, home.cfgs[0].DisplayOrder)
	}
}

// fakeHistory records what the seed asked the playback service to write.
type fakeHistory struct {
	resumed map[string]int64
	watched []string
}

func (f *fakeHistory) ReportProgress(_ context.Context, _ string, mediaItemID string, positionMs int64, _ bool) error {
	if f.resumed == nil {
		f.resumed = map[string]int64{}
	}
	f.resumed[mediaItemID] = positionMs
	return nil
}

func (f *fakeHistory) MarkAsWatched(_ context.Context, _ string, mediaItemID string) error {
	f.watched = append(f.watched, mediaItemID)
	return nil
}

// The seeded history: a film watched, a film resumed at its share of the
// duration, the first episodes of a series watched (so Next up offers the
// following one); a title not indexed yet is skipped; a title that already
// has progress is left alone at the next boot.
func TestSeedHistory_ResumesWatchesAndSkipsWhatExists(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if err := h.boot.EnsureAdmin(ctx); err != nil {
		t.Fatal(err)
	}
	libs, _, err := h.boot.EnsureLibraries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	media := libfakes.NewFakeMediaItemStore()
	progress := pbfakes.NewFakeWatchProgressRepository()
	hist := &fakeHistory{}
	h.boot.WithHistory(media, progress, hist)

	tenMin, _ := shared.NewDurationMs(10 * 60 * 1000)
	film := func(id, title string) *library.MediaItem {
		return &library.MediaItem{ID: shared.MediaItemID(id), LibraryID: libs["movies"], Type: library.MovieItem, Title: title,
			File: &library.MediaFile{Path: "/m/" + id, Duration: tenMin}}
	}
	_ = media.Create(ctx, film("bbb", "Big Buck Bunny"))
	_ = media.Create(ctx, film("sintel", "Sintel"))
	// Spring is not downloaded yet: skipped, no error.
	_ = media.Create(ctx, &library.MediaItem{ID: "bleep", LibraryID: libs["series"], Type: library.Series, Title: "Colonel Bleep"})
	one := 1
	_ = media.Create(ctx, &library.MediaItem{ID: "bleep-s1", ParentID: "bleep", Type: library.Season, Title: "Season 1", IndexNumber: &one})
	for i := 1; i <= 3; i++ {
		n := i
		_ = media.Create(ctx, &library.MediaItem{ID: shared.MediaItemID(fmt.Sprintf("bleep-e%d", i)), ParentID: "bleep-s1", Type: library.Episode, Title: fmt.Sprintf("E%d", i), IndexNumber: &n})
	}

	if err := h.boot.SeedHistory(ctx, libs); err != nil {
		t.Fatal(err)
	}
	if got := hist.resumed["sintel"]; got != 4*60*1000 {
		t.Errorf("Sintel resumed at %d ms, want 40%% of ten minutes", got)
	}
	wantWatched := []string{"bbb", "bleep-e1", "bleep-e2"}
	if strings.Join(hist.watched, ",") != strings.Join(wantWatched, ",") {
		t.Errorf("watched = %v, want %v", hist.watched, wantWatched)
	}

	// Next boot: the titles have progress on the demo profile (the seed's,
	// or a visitor's who moved on), nothing is rewritten.
	hist.resumed, hist.watched = nil, nil
	admin, _ := h.users.GetByUsername(ctx, demo.Username)
	profs, _ := h.profiles.GetByUserID(ctx, admin.ID)
	pid := profs[0].ID
	for _, id := range []string{"sintel", "bbb", "bleep-e1", "bleep-e2"} {
		_ = progress.Upsert(ctx, &playback.WatchProgress{ID: "wp-" + id, ProfileID: pid, MediaItemID: shared.MediaItemID(id), Completed: id != "sintel"})
	}
	if err := h.boot.SeedHistory(ctx, libs); err != nil {
		t.Fatal(err)
	}
	if len(hist.resumed) != 0 || len(hist.watched) != 0 {
		t.Errorf("second boot rewrote history: resumed=%v watched=%v", hist.resumed, hist.watched)
	}
}

// A demo torn down then booted again (the flag cycled, or a second server
// without the flag on the same database) left `demo` with the teardown's
// random password: the next bootstrap must give the published one back.
func TestEnsureAdmin_RestoresThePublishedPasswordAfterATeardown(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if err := h.boot.EnsureAdmin(ctx); err != nil {
		t.Fatal(err)
	}
	if err := h.boot.Teardown(ctx); err != nil {
		t.Fatal(err)
	}
	u, _ := h.users.GetByUsername(ctx, demo.Username)
	hasher := ufakes.NewFakePasswordHasher()
	if ok, _ := hasher.Verify(demo.Password, u.PasswordHash); ok {
		t.Fatal("precondition: the teardown rotates the last admin's password")
	}
	if err := h.boot.EnsureAdmin(ctx); err != nil {
		t.Fatal(err)
	}
	u, _ = h.users.GetByUsername(ctx, demo.Username)
	if ok, _ := hasher.Verify(demo.Password, u.PasswordHash); !ok {
		t.Error("the published demo password was not restored")
	}
	if !u.IsAdmin || !u.Enabled {
		t.Error("demo must be an enabled administrator")
	}
}

type fakeStreams struct{ active int }

func (f *fakeStreams) GetServerStats(context.Context) (*activity.ServerStatsResult, error) {
	return &activity.ServerStatsResult{ActiveStreams: f.active}, nil
}

// The in-server reset: nothing while someone watches, unless the daily floor
// is due; when it runs, every demo profile's progress is cleared and the seed
// is re-asserted - here, the published password a teardown had rotated.
func TestResetIfIdle_WaitsForAnIdleServerButHonoursTheFloor(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if err := h.boot.EnsureAdmin(ctx); err != nil {
		t.Fatal(err)
	}
	progress := pbfakes.NewFakeWatchProgressRepository()
	streams := &fakeStreams{active: 1}
	h.boot.WithReset(streams, progress)

	admin, _ := h.users.GetByUsername(ctx, demo.Username)
	profs, _ := h.profiles.GetByUserID(ctx, admin.ID)
	pid := profs[0].ID
	_ = progress.Upsert(ctx, &playback.WatchProgress{ID: "wp-1", ProfileID: pid, MediaItemID: "m1"})

	done, err := h.boot.ResetIfIdle(ctx, false)
	if err != nil || done {
		t.Fatalf("reset under a viewer: done=%v err=%v", done, err)
	}
	if wp, _ := progress.GetByProfileAndMedia(ctx, string(pid), "m1"); wp == nil {
		t.Fatal("progress cleared while someone was watching")
	}

	done, err = h.boot.ResetIfIdle(ctx, true)
	if err != nil || !done {
		t.Fatalf("daily floor: done=%v err=%v", done, err)
	}
	if wp, _ := progress.GetByProfileAndMedia(ctx, string(pid), "m1"); wp != nil {
		t.Error("the floor reset kept the visitor's progress")
	}

	streams.active = 0
	_ = progress.Upsert(ctx, &playback.WatchProgress{ID: "wp-2", ProfileID: pid, MediaItemID: "m2"})
	admin.PasswordHash = "rotated"
	_ = h.users.Update(ctx, admin)
	done, err = h.boot.ResetIfIdle(ctx, false)
	if err != nil || !done {
		t.Fatalf("idle reset: done=%v err=%v", done, err)
	}
	if wp, _ := progress.GetByProfileAndMedia(ctx, string(pid), "m2"); wp != nil {
		t.Error("the idle reset kept the visitor's progress")
	}
	u, _ := h.users.GetByUsername(ctx, demo.Username)
	if ok, _ := ufakes.NewFakePasswordHasher().Verify(demo.Password, u.PasswordHash); !ok {
		t.Error("the reset did not restore the published password")
	}
}

// The public demo still carries "Demo - Films" & co: the next boot renames
// those libraries (same id, nothing recreated), a second boot changes nothing,
// and an operator's own library that only shares an old name is left alone.
func TestEnsureLibraries_RenamesTheFormerDemoLibrariesInPlace(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	old, err := library.NewLibrary("lib-old-films", "Demo - Films", library.LibraryType("movie"),
		library.WithLibraryPaths([]string{filepath.Join(h.dir, "movies")}))
	if err != nil {
		t.Fatal(err)
	}
	stranger, err := library.NewLibrary("lib-mine", "Demo - Séries", library.LibraryType("tvshow"),
		library.WithLibraryPaths([]string{"/media/my-series"}))
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range []*library.Library{old, stranger} {
		if createErr := h.libs.repo.Create(ctx, l); createErr != nil {
			t.Fatal(createErr)
		}
	}

	libs, created, err := h.boot.EnsureLibraries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if libs["movies"] != "lib-old-films" {
		t.Fatalf("movies -> %s, want the renamed lib-old-films", libs["movies"])
	}
	for _, c := range created {
		if c == "movies" {
			t.Fatal("a second movies library was created next to the old one")
		}
	}
	got, _ := h.libs.repo.GetByID(ctx, "lib-old-films")
	if got == nil || got.Name != "Movies" {
		t.Fatalf("old films library now named %v, want Movies", got)
	}
	mine, _ := h.libs.repo.GetByID(ctx, "lib-mine")
	if mine == nil || mine.Name != "Demo - Séries" {
		t.Fatalf("the operator's own library was touched: %v", mine)
	}

	before, _ := h.libs.repo.List(ctx, library.ListLibrariesFilter{})
	if _, again, err := h.boot.EnsureLibraries(ctx); err != nil || len(again) != 0 {
		t.Fatalf("second run: created %v, err %v - want nothing", again, err)
	}
	after, _ := h.libs.repo.List(ctx, library.ListLibrariesFilter{})
	if len(after) != len(before) {
		t.Fatalf("second run changed the library count %d -> %d", len(before), len(after))
	}
}
