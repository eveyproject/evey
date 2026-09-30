// Package demo makes a demo instance self-sufficient: with EVEY_DEMO=1 a fresh
// server boots into a working demo - the `demo` administrator, the visitor
// accounts and their profiles, the four libraries pointed at the demo media
// directory, and the freely licensed media downloaded from the embedded
// manifest (internal/pkg/demo). Every step is idempotent: what exists is left
// alone, so the bootstrap runs at every boot and after the host restore.
//
// It only composes the product's own services (Setup, CreateUser,
// CreateLibrary, ScanLibrary): nothing here is a demo-only business rule.
package demo

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/feed"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/playback"
	"github.com/evey-project/evey/internal/domain/plugin"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/demo"
	"github.com/evey-project/evey/internal/pkg/id"
)

// AdminSetup is the wizard's path: the first administrator of an instance.
type AdminSetup interface {
	Setup(ctx context.Context, username, password, email string, opts ...appuser.SetupOption) (*user.User, error)
}

// Accounts is the slice of the user service the seed needs.
type Accounts interface {
	CreateUser(ctx context.Context, callerIsAdmin bool, u *user.User, password string) error
	CreateProfile(ctx context.Context, p *user.Profile) error
	UpdateProfile(ctx context.Context, p *user.Profile) error
	DeleteUser(ctx context.Context, userID string) error
}

// Libraries is the slice of the library service the bootstrap needs.
type Libraries interface {
	CreateLibrary(ctx context.Context, lib *library.Library) error
	UpdateLibrary(ctx context.Context, lib *library.Library) error
	ScanLibrary(ctx context.Context, libraryID string) (added, updated int, err error)
	DeleteLibrary(ctx context.Context, libraryID string) error
}

// PluginInstaller and PluginRuntime are the slices of the two plugin services
// the bootstrap needs: install/uninstall a bundle, enable it and set its config.
type PluginInstaller interface {
	InstallPlugin(ctx context.Context, source string) (*plugin.Plugin, error)
	UninstallPlugin(ctx context.Context, id string) error
}

type PluginRuntime interface {
	EnablePlugin(ctx context.Context, id string) error
	UpdatePluginConfig(ctx context.Context, id string, config map[string]any) error
}

// HomeLayout is the slice of the feed service that writes the admin home
// configuration (the default home page every visitor starts from).
type HomeLayout interface {
	BatchUpdateAdminConfigs(ctx context.Context, cfgs []*feed.AdminConfig) error
}

// History is the slice of the playback service that records what a profile
// watched: a resume position, or a title watched to the end.
type History interface {
	ReportProgress(ctx context.Context, profileID, mediaItemID string, positionMs int64, completed bool) error
	MarkAsWatched(ctx context.Context, profileID, mediaItemID string) error
}

// MarkerKey is the server-config entry the bootstrap leaves behind, so a boot
// WITHOUT the demo flag knows this instance was a demo and cleans up after it.
// An instance that never ran the demo has no marker and is never touched.
const MarkerKey = "demo.bootstrapped"

type Bootstrapper struct {
	users    user.UserRepository
	profiles user.ProfileRepository
	setup    AdminSetup
	accounts Accounts
	access   user.LibraryAccessRepository // optional
	libs     library.LibraryQueryRepo
	libSvc   Libraries
	hasher   user.PasswordHasher
	marker   shared.ServerConfigRepository // optional
	mediaDir string
	client   *http.Client
	log      *slog.Logger

	// Optional plugin step (WithPlugins): nil leaves the demo without plugins.
	plugins   plugin.PluginRepository
	installer PluginInstaller
	runtime   PluginRuntime
	pluginDir string
	home      HomeLayout // optional

	// Optional history step (WithHistory): seeds pkg/demo.History.
	media    library.MediaItemQueryRepo
	progress playback.ProgressQueryRepo
	history  History

	// Optional periodic reset (WithReset, reset.go).
	streams       ActiveStreams
	progressReset ProgressReset
}

func NewBootstrapper(users user.UserRepository, profiles user.ProfileRepository, setup AdminSetup, accounts Accounts,
	libs library.LibraryQueryRepo, libSvc Libraries, hasher user.PasswordHasher, mediaDir string, log *slog.Logger) *Bootstrapper {
	if log == nil {
		log = slog.Default()
	}
	return &Bootstrapper{
		users: users, profiles: profiles, setup: setup, accounts: accounts,
		libs: libs, libSvc: libSvc, hasher: hasher, mediaDir: mediaDir,
		// One film is over a gigabyte on a slow mirror: the per-request
		// deadline is generous, the caller's context bounds the whole run.
		client: &http.Client{Timeout: 2 * time.Hour},
		log:    log,
	}
}

// WithLibraryAccess confines the seeded accounts to the demo libraries.
func (b *Bootstrapper) WithLibraryAccess(access user.LibraryAccessRepository) *Bootstrapper {
	b.access = access
	return b
}

// WithMarker records the bootstrap in the server config, which is what lets a
// later non-demo boot tear the demo down.
func (b *Bootstrapper) WithMarker(cfg shared.ServerConfigRepository) *Bootstrapper {
	b.marker = cfg
	return b
}

// WithPlugins lets the bootstrap install the demo plugins (pkg/demo.Plugins)
// from the bundles found under pluginDir, and the teardown remove them.
func (b *Bootstrapper) WithPlugins(repo plugin.PluginRepository, installer PluginInstaller, runtime PluginRuntime, pluginDir string) *Bootstrapper {
	b.plugins, b.installer, b.runtime, b.pluginDir = repo, installer, runtime, pluginDir
	return b
}

// WithHomeLayout lets the bootstrap arrange the demo's home page (pkg/demo.Home).
func (b *Bootstrapper) WithHomeLayout(home HomeLayout) *Bootstrapper {
	b.home = home
	return b
}

// WithHistory lets the bootstrap seed the demo profile's watch history
// (pkg/demo.History) once the media are indexed.
func (b *Bootstrapper) WithHistory(media library.MediaItemQueryRepo, progress playback.ProgressQueryRepo, history History) *Bootstrapper {
	b.media, b.progress, b.history = media, progress, history
	return b
}

// WithHTTPClient replaces the downloader's client (tests).
func (b *Bootstrapper) WithHTTPClient(c *http.Client) *Bootstrapper {
	b.client = c
	return b
}

// Run performs the whole bootstrap: accounts, libraries, then the media
// downloads, scanning each library as its files land. It is meant for a
// detached goroutine bounded by the server context; the accounts and
// libraries are ready within seconds, the media takes as long as the mirrors
// allow.
func (b *Bootstrapper) Run(ctx context.Context) error {
	if err := b.EnsureAdmin(ctx); err != nil {
		return err
	}
	if err := b.SeedAccounts(ctx); err != nil {
		return err
	}
	libs, created, err := b.EnsureLibraries(ctx)
	if err != nil {
		return err
	}
	if err := b.ConfineAccounts(ctx, libs); err != nil {
		return err
	}
	if err := b.MarkBootstrapped(ctx); err != nil {
		return err
	}
	plugins, perr := b.EnsurePlugins(ctx)
	if perr != nil {
		return perr
	}
	if herr := b.ArrangeHome(ctx, libs, plugins); herr != nil {
		return herr
	}
	// A library created just now was not there when scan-on-startup ran, and
	// its folder may already hold the media (a restore, a second database on
	// the same disk): index what is there before the downloads start.
	for _, folder := range created {
		if _, _, err := b.libSvc.ScanLibrary(ctx, string(libs[folder])); err != nil {
			b.log.Warn("demo: initial scan", "library", folder, "err", err)
		}
	}
	if err := b.DownloadMedia(ctx, libs); err != nil {
		return err
	}
	// Last: the history names titles, which exist only once downloaded and
	// scanned. A title still missing is seeded at the next boot.
	return b.SeedHistory(ctx, libs)
}

// MarkBootstrapped leaves the marker Teardown looks for.
func (b *Bootstrapper) MarkBootstrapped(ctx context.Context) error {
	if b.marker == nil {
		return nil
	}
	return b.marker.Set(ctx, MarkerKey, time.Now().UTC().Format(time.RFC3339))
}

// WasBootstrapped reports whether a previous boot ran the demo bootstrap here.
func (b *Bootstrapper) WasBootstrapped(ctx context.Context) bool {
	if b.marker == nil {
		return false
	}
	e, err := b.marker.Get(ctx, MarkerKey)
	return err == nil && e != nil && e.Value != ""
}

// Teardown undoes the bootstrap when the demo flag is gone: the demo
// libraries (and their indexed items), the visitor accounts, the `demo`
// administrator, the manifest's files on disk, then the marker. Only what the
// bootstrap created is touched - libraries by their demo names, accounts by
// their seed usernames, files by their manifest paths - so an operator's own
// data next to it survives.
//
// The one thing it will not do is leave the instance without an
// administrator: when `demo` is the last one, its published password is
// replaced by a random one and the log says how to recover, instead of
// deleting the account and reopening the setup wizard on a live server.
func (b *Bootstrapper) Teardown(ctx context.Context) error {
	existing, err := b.libs.List(ctx, library.ListLibrariesFilter{})
	if err != nil {
		return fmt.Errorf("demo teardown: list libraries: %w", err)
	}
	names := map[string]bool{}
	for _, spec := range demo.Libraries {
		names[spec.Name] = true
		for _, former := range spec.FormerNames {
			names[former] = true
		}
	}
	for _, l := range existing {
		if !names[l.Name] {
			continue
		}
		if err := b.libSvc.DeleteLibrary(ctx, string(l.ID)); err != nil {
			return fmt.Errorf("demo teardown: delete library %s: %w", l.Name, err)
		}
		b.log.Info("demo: library removed", "name", l.Name)
	}

	for _, a := range seedAccounts {
		u, err := b.users.GetByUsername(ctx, a.username)
		if err != nil || u == nil {
			continue
		}
		if err := b.accounts.DeleteUser(ctx, string(u.ID)); err != nil {
			return fmt.Errorf("demo teardown: delete %s: %w", a.username, err)
		}
		b.log.Info("demo: account removed", "username", a.username)
	}
	if u, err := b.users.GetByUsername(ctx, demo.Username); err == nil && u != nil {
		switch err := b.accounts.DeleteUser(ctx, string(u.ID)); {
		case err == nil:
			b.log.Info("demo: administrator removed", "username", demo.Username)
		case errors.Is(err, appuser.ErrLastAdmin):
			pw, perr := randomPassword()
			if perr != nil {
				return perr
			}
			hash, herr := b.hasher.Hash(pw)
			if herr != nil {
				return herr
			}
			u.PasswordHash = hash
			u.UpdatedAt = time.Now()
			if uerr := b.users.Update(ctx, u); uerr != nil {
				return fmt.Errorf("demo teardown: rotate last admin password: %w", uerr)
			}
			b.log.Warn("demo: `demo` is the only administrator, kept with a NEW RANDOM password - "+
				"recover with `evey admin reset-password --user demo@demo.invalid`, or create your own administrator and delete it",
				"username", demo.Username)
		default:
			return fmt.Errorf("demo teardown: delete %s: %w", demo.Username, err)
		}
	}

	for _, f := range demo.Manifest {
		dest := filepath.Join(b.mediaDir, filepath.FromSlash(f.Path))
		for _, p := range []string{dest, dest + ".part", dest + ".unzip"} {
			if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
				b.log.Warn("demo: remove file", "path", p, "err", err)
			}
		}
	}
	// Folders the manifest created, deepest first; only when empty, so a file
	// the operator dropped there is not lost.
	var dirs []string
	for _, f := range demo.Manifest {
		d := filepath.Dir(filepath.Join(b.mediaDir, filepath.FromSlash(f.Path)))
		for d != b.mediaDir && len(d) > len(b.mediaDir) {
			dirs = append(dirs, d)
			d = filepath.Dir(d)
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, d := range dirs {
		b.removeIfEmpty(d)
	}
	for _, spec := range demo.Libraries {
		b.removeIfEmpty(filepath.Join(b.mediaDir, spec.Folder))
	}

	if err := b.RemovePlugins(ctx); err != nil {
		return err
	}

	if b.marker != nil {
		if err := b.marker.Delete(ctx, MarkerKey); err != nil {
			return fmt.Errorf("demo teardown: clear marker: %w", err)
		}
	}
	b.log.Info("demo: teardown complete")
	return nil
}

// EnsureAdmin makes sure the published `demo` / `demo` administrator can sign
// in: created through the wizard's Setup when the instance has no
// administrator yet, as an extra administrator otherwise (a dev box), and
// re-enabled if something disabled it - with the PUBLISHED password: a
// teardown (a boot without the flag on the same database, 2026-09-29) keeps
// `demo` with a random one, and re-enabling it without the password left the
// demo locking every visitor out.
func (b *Bootstrapper) EnsureAdmin(ctx context.Context) error {
	if existing, err := b.users.GetByUsername(ctx, demo.Username); err == nil && existing != nil {
		pwOK, verr := b.hasher.Verify(demo.Password, existing.PasswordHash)
		if verr != nil {
			pwOK = false
		}
		if existing.IsAdmin && existing.Enabled && pwOK {
			return nil
		}
		if !pwOK {
			hash, herr := b.hasher.Hash(demo.Password)
			if herr != nil {
				return fmt.Errorf("demo admin: hash published password: %w", herr)
			}
			existing.PasswordHash = hash
			existing.UpdatedAt = time.Now()
		}
		existing.IsAdmin = true
		existing.Enabled = true
		if err := b.users.Update(ctx, existing); err != nil {
			return fmt.Errorf("demo admin: restore: %w", err)
		}
		b.log.Info("demo: administrator restored", "username", demo.Username)
		return nil
	}
	admins, err := b.users.CountAdmins(ctx)
	if err != nil {
		return fmt.Errorf("demo admin: count admins: %w", err)
	}
	if admins == 0 {
		if _, serr := b.setup.Setup(ctx, demo.Username, demo.Password, "demo@demo.invalid", appuser.WithSetupLocale("en")); serr != nil {
			return fmt.Errorf("demo admin: setup: %w", serr)
		}
		b.log.Info("demo: administrator created through setup", "username", demo.Username)
		return nil
	}
	u, err := user.NewUser(demo.Username, "demo@demo.invalid", "pending")
	if err != nil {
		return err
	}
	u.IsAdmin = true
	if err := b.accounts.CreateUser(ctx, true, u, demo.Password); err != nil {
		return fmt.Errorf("demo admin: create: %w", err)
	}
	b.log.Info("demo: administrator created", "username", demo.Username)
	return nil
}

// EnsureLibraries creates the missing demo libraries, each on its folder
// under the media directory, and returns folder -> library id for all four
// plus the folders whose library was created by this call.
func (b *Bootstrapper) EnsureLibraries(ctx context.Context) (map[string]shared.LibraryID, []string, error) {
	existing, err := b.libs.List(ctx, library.ListLibrariesFilter{})
	if err != nil {
		return nil, nil, fmt.Errorf("demo libraries: list: %w", err)
	}
	byName := map[string]*library.Library{}
	for _, l := range existing {
		byName[l.Name] = l
	}
	out := make(map[string]shared.LibraryID, len(demo.Libraries))
	var created []string
	for _, spec := range demo.Libraries {
		dir := filepath.Join(b.mediaDir, spec.Folder)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, nil, fmt.Errorf("demo libraries: mkdir %s: %w", dir, err)
		}
		if l, ok := byName[spec.Name]; ok {
			out[spec.Folder] = l.ID
			continue
		}
		if l := formerLibrary(byName, spec, dir); l != nil {
			old := l.Name
			l.Name = spec.Name
			if err := b.libSvc.UpdateLibrary(ctx, l); err != nil {
				return nil, nil, fmt.Errorf("demo libraries: rename %s: %w", old, err)
			}
			b.log.Info("demo: library renamed", "from", old, "to", spec.Name)
			out[spec.Folder] = l.ID
			continue
		}
		lib, err := library.NewLibrary(shared.LibraryID(id.New("lib-")), spec.Name, library.LibraryType(spec.Type),
			library.WithLibraryPaths([]string{dir}),
			library.WithLibraryScanOnStartup(true),
			library.WithLibraryRealtimeMonitoring(false))
		if err != nil {
			return nil, nil, fmt.Errorf("demo libraries: %s: %w", spec.Name, err)
		}
		if err := b.libSvc.CreateLibrary(ctx, lib); err != nil {
			return nil, nil, fmt.Errorf("demo libraries: create %s: %w", spec.Name, err)
		}
		b.log.Info("demo: library created", "name", spec.Name, "path", dir)
		out[spec.Folder] = lib.ID
		created = append(created, spec.Folder)
	}
	return out, created, nil
}

// formerLibrary finds the library a previous release created for spec: one of
// its former names AND pointed at the demo folder, so an operator's own
// library that happens to share the name is never taken over. Idempotent: once
// renamed, the library is found by its current name.
func formerLibrary(byName map[string]*library.Library, spec demo.Library, dir string) *library.Library {
	for _, former := range spec.FormerNames {
		l, ok := byName[former]
		if !ok {
			continue
		}
		for _, p := range l.Paths {
			if filepath.Clean(p) == filepath.Clean(dir) {
				return l
			}
		}
	}
	return nil
}

// DownloadMedia fetches every manifest file that is missing, folder by folder
// in manifest order, and scans a library once a download landed in it. A file
// that fails is logged and skipped: the next boot retries it.
func (b *Bootstrapper) DownloadMedia(ctx context.Context, libs map[string]shared.LibraryID) error {
	return b.DownloadManifest(ctx, demo.Manifest, libs)
}

// DownloadManifest is DownloadMedia over an explicit list (tests).
func (b *Bootstrapper) DownloadManifest(ctx context.Context, manifest []demo.MediaFile, libs map[string]shared.LibraryID) error {
	touched := map[string]bool{}
	current := ""
	flush := func() {
		if current == "" || !touched[current] {
			return
		}
		if libID, ok := libs[current]; ok {
			if _, _, err := b.libSvc.ScanLibrary(ctx, string(libID)); err != nil {
				b.log.Warn("demo: scan after download", "library", current, "err", err)
			} else {
				b.log.Info("demo: library scanned", "library", current)
			}
		}
	}
	for _, f := range manifest {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		folder := topFolder(f.Path)
		if folder != current {
			flush()
			current = folder
		}
		dest := filepath.Join(b.mediaDir, filepath.FromSlash(f.Path))
		if st, err := os.Stat(dest); err == nil && st.Size() > 0 {
			continue
		}
		b.log.Info("demo: downloading", "file", f.Path)
		if err := b.fetch(ctx, f, dest); err != nil {
			b.log.Warn("demo: download failed, will retry at next boot", "file", f.Path, "err", err)
			continue
		}
		touched[folder] = true
	}
	flush()
	return nil
}

func topFolder(rel string) string {
	for i := range len(rel) {
		if rel[i] == '/' {
			return rel[:i]
		}
	}
	return rel
}

// fetch downloads f to dest through a .part file, so an interrupted download
// never masquerades as a media file, extracting the zip member when the URL
// is an archive.
func (b *Bootstrapper) fetch(ctx context.Context, f demo.MediaFile, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Evey-demo-bootstrap/1.0 (+https://eveyproject.org)")
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: HTTP %d", f.URL, resp.StatusCode)
	}
	part := dest + ".part"
	out, err := os.Create(part)
	if err != nil {
		return err
	}
	n, copyErr := io.Copy(out, resp.Body)
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil || n == 0 {
		b.removeQuietly(part)
		switch {
		case copyErr != nil:
			return copyErr
		case closeErr != nil:
			return fmt.Errorf("close download: %w", closeErr)
		default:
			return fmt.Errorf("empty download (%d bytes)", n)
		}
	}
	if f.ZipMember != "" {
		if err := extractZipMember(part, f.ZipMember, dest); err != nil {
			b.removeQuietly(part)
			return err
		}
		return os.Remove(part)
	}
	return os.Rename(part, dest)
}

// removeQuietly deletes a temporary file; a failure is logged, never fatal.
func (b *Bootstrapper) removeQuietly(p string) {
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		b.log.Warn("demo: remove temporary file", "path", p, "err", err)
	}
}

// removeIfEmpty deletes a folder the bootstrap created, only when nothing is
// left in it: a non-empty folder fails to remove, which is the point.
func (b *Bootstrapper) removeIfEmpty(d string) {
	if err := os.Remove(d); err != nil && !errors.Is(err, os.ErrNotExist) {
		b.log.Debug("demo: folder kept", "path", d, "reason", err.Error())
	}
}

func randomPassword() (string, error) {
	buf := make([]byte, 24)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// EnsurePlugins installs each demo plugin whose bundle sits under the plugin
// directory, enables it and (re)applies the manifest's config - at every boot,
// so a visitor cannot leave it in another state and a restore shows the demo
// as designed. A bundle that is missing is logged and skipped: the demo works
// without its plugins, the operator sees what to drop where. No-op unless
// WithPlugins was given.
func (b *Bootstrapper) EnsurePlugins(ctx context.Context) (map[string]string, error) {
	ids := map[string]string{}
	if b.plugins == nil || b.installer == nil || b.runtime == nil {
		return ids, nil
	}
	// A plugin an earlier demo seeded and this one replaced goes first, so
	// the home never shows both.
	for _, name := range demo.RetiredPlugins {
		if p, err := b.plugins.GetByName(ctx, name); err == nil && p != nil {
			if err := b.installer.UninstallPlugin(ctx, p.ID); err != nil {
				return nil, fmt.Errorf("demo: uninstall retired plugin %s: %w", name, err)
			}
			b.log.Info("demo: retired plugin removed", "plugin", name)
		}
	}
	for _, spec := range demo.Plugins {
		p, err := b.plugins.GetByName(ctx, spec.Name)
		if err != nil && !errors.Is(err, shared.ErrNotFound) {
			return nil, fmt.Errorf("demo: look up plugin %s: %w", spec.Name, err)
		}
		if p == nil {
			bundle := filepath.Join(b.pluginDir, spec.File)
			if _, statErr := os.Stat(bundle); statErr != nil {
				b.log.Warn("demo: plugin bundle absent, plugin skipped - build it from examples/plugins and drop it there",
					"plugin", spec.Name, "bundle", bundle)
				continue
			}
			if p, err = b.installer.InstallPlugin(ctx, bundle); err != nil {
				return nil, fmt.Errorf("demo: install plugin %s: %w", spec.Name, err)
			}
			b.log.Info("demo: plugin installed", "plugin", spec.Name, "version", p.Version)
		}
		ids[spec.Name] = p.ID
		if cfgErr := b.runtime.UpdatePluginConfig(ctx, p.ID, spec.Config); cfgErr != nil {
			return nil, fmt.Errorf("demo: configure plugin %s: %w", spec.Name, cfgErr)
		}
		// Re-read before judging, and judge the flag only: the install
		// enables and starts the plugin, but the row says "running" only once
		// the handshake lands, so a status check right after would enable
		// again and be refused ("already running"). An enabled plugin that
		// is not running is the runtime's business, not the seed's.
		if p, err = b.plugins.GetByName(ctx, spec.Name); err != nil {
			return nil, fmt.Errorf("demo: re-read plugin %s: %w", spec.Name, err)
		}
		if !p.Enabled {
			if enErr := b.runtime.EnablePlugin(ctx, p.ID); enErr != nil {
				return nil, fmt.Errorf("demo: enable plugin %s: %w", spec.Name, enErr)
			}
		}
	}
	return ids, nil
}

// ArrangeHome writes the demo's default home page (pkg/demo.Home) as the admin
// feed configuration: the listed sections in that order, every other known
// section and the other demo libraries disabled. Written at every boot, so a
// restore or an earlier layout never shows another order. A section whose
// library or plugin is not there (bundle absent) is skipped. No-op unless
// WithHomeLayout was given.
func (b *Bootstrapper) ArrangeHome(ctx context.Context, libs map[string]shared.LibraryID, plugins map[string]string) error {
	if b.home == nil {
		return nil
	}
	libByName := map[string]shared.LibraryID{}
	for _, spec := range demo.Libraries {
		if id, ok := libs[spec.Folder]; ok {
			libByName[spec.Name] = id
		}
	}
	shown := map[feed.Type]bool{}
	var cfgs []*feed.AdminConfig
	for _, sec := range demo.Home {
		var ft feed.Type
		params := map[string]any{}
		switch {
		case sec.Plugin != "":
			id, ok := plugins[sec.Plugin]
			if !ok {
				continue
			}
			ft = feed.PluginType(id)
		case sec.Library != "":
			id, ok := libByName[sec.Library]
			if !ok {
				continue
			}
			ft = feed.LibraryType(string(id))
			params["libraryName"] = sec.Library
		default:
			ft = feed.Type(sec.Type)
		}
		shown[ft] = true
		cfgs = append(cfgs, &feed.AdminConfig{FeedType: ft, Enabled: true, DisplayOrder: len(cfgs), MaxItems: 20, Params: params})
	}
	order := len(cfgs)
	for _, ft := range demo.HiddenHomeTypes {
		if shown[feed.Type(ft)] {
			continue
		}
		cfgs = append(cfgs, &feed.AdminConfig{FeedType: feed.Type(ft), Enabled: false, DisplayOrder: order, MaxItems: 20})
		order++
	}
	for _, spec := range demo.Libraries {
		id, ok := libByName[spec.Name]
		if !ok || shown[feed.LibraryType(string(id))] {
			continue
		}
		cfgs = append(cfgs, &feed.AdminConfig{FeedType: feed.LibraryType(string(id)), Enabled: false, DisplayOrder: order, MaxItems: 20,
			Params: map[string]any{"libraryName": spec.Name}})
		order++
	}
	if err := b.home.BatchUpdateAdminConfigs(ctx, cfgs); err != nil {
		return fmt.Errorf("demo: arrange home: %w", err)
	}
	return nil
}

// RemovePlugins uninstalls the demo plugins, by name; one the operator never
// had is left alone. No-op unless WithPlugins was given.
func (b *Bootstrapper) RemovePlugins(ctx context.Context) error {
	if b.plugins == nil || b.installer == nil {
		return nil
	}
	for _, spec := range demo.Plugins {
		p, err := b.plugins.GetByName(ctx, spec.Name)
		if err != nil || p == nil {
			continue
		}
		if err := b.installer.UninstallPlugin(ctx, p.ID); err != nil {
			return fmt.Errorf("demo teardown: uninstall plugin %s: %w", spec.Name, err)
		}
		b.log.Info("demo: plugin removed", "plugin", spec.Name)
	}
	return nil
}

// SeedHistory records pkg/demo.History on the demo administrator's default
// profile: films resumed part-way or watched, the first episodes of the
// series watched. Idempotent - a title that already has progress (seeded, or
// watched by a visitor since) is left alone - and tolerant: a title not
// indexed yet is skipped and picked up at the next boot. No-op unless
// WithHistory was given.
func (b *Bootstrapper) SeedHistory(ctx context.Context, libs map[string]shared.LibraryID) error {
	if b.media == nil || b.progress == nil || b.history == nil {
		return nil
	}
	admin, err := b.users.GetByUsername(ctx, demo.Username)
	if err != nil || admin == nil {
		return fmt.Errorf("demo: seed history: demo account: %w", err)
	}
	profiles, err := b.profiles.GetByUserID(ctx, admin.ID)
	if err != nil || len(profiles) == 0 {
		return fmt.Errorf("demo: seed history: profiles of %s: %w", demo.Username, err)
	}
	profileID := string(profiles[0].ID)
	for _, p := range profiles {
		if p.IsDefault {
			profileID = string(p.ID)
		}
	}
	for _, h := range demo.History {
		switch {
		case h.Film != "":
			item := b.findTitle(ctx, libs["movies"], library.MovieItem, h.Film)
			if item == nil {
				continue
			}
			if err := b.seedOne(ctx, profileID, item, h.Percent); err != nil {
				return err
			}
		case h.Series != "":
			series := b.findTitle(ctx, libs["series"], library.Series, h.Series)
			if series == nil {
				continue
			}
			for _, ep := range b.firstSeasonEpisodes(ctx, series) {
				if ep.IndexNumber == nil || *ep.IndexNumber > h.WatchedEpisodes {
					continue
				}
				if err := b.seedOne(ctx, profileID, ep, 0); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// seedOne writes one title's progress unless the profile already has some.
// percent 0 marks it watched; otherwise the position is that share of the
// file's duration (a title whose duration is unknown is marked watched).
func (b *Bootstrapper) seedOne(ctx context.Context, profileID string, item *library.MediaItem, percent int) error {
	if existing, err := b.progress.GetByProfileAndMedia(ctx, profileID, string(item.ID)); err == nil && existing != nil {
		return nil
	}
	var durationMs int64
	if item.File != nil {
		durationMs = int64(item.File.Duration.Seconds() * 1000)
	}
	if percent <= 0 || durationMs <= 0 {
		if err := b.history.MarkAsWatched(ctx, profileID, string(item.ID)); err != nil {
			return fmt.Errorf("demo: seed history: mark %s watched: %w", item.Title, err)
		}
	} else if err := b.history.ReportProgress(ctx, profileID, string(item.ID), durationMs*int64(percent)/100, false); err != nil {
		return fmt.Errorf("demo: seed history: resume %s: %w", item.Title, err)
	}
	b.log.Info("demo: history seeded", "title", item.Title, "percent", percent)
	return nil
}

// findTitle returns the item of that type in the library whose title (or
// scan title) matches, case-insensitively; nil when not indexed yet.
func (b *Bootstrapper) findTitle(ctx context.Context, libID shared.LibraryID, typ library.MediaItemType, title string) *library.MediaItem {
	if libID == "" {
		return nil
	}
	t := string(typ)
	items, err := b.media.List(ctx, library.ListMediaItemsFilter{LibraryID: &libID, Type: &t, Limit: 500})
	if err != nil {
		b.log.Warn("demo: seed history: list", "type", t, "err", err)
		return nil
	}
	for _, it := range items {
		if strings.EqualFold(it.Title, title) || strings.EqualFold(it.ScanTitle, title) {
			return it
		}
	}
	return nil
}

// firstSeasonEpisodes lists the episodes of the series' season 1, in order.
func (b *Bootstrapper) firstSeasonEpisodes(ctx context.Context, series *library.MediaItem) []*library.MediaItem {
	seriesID, seasonType, episodeType := string(series.ID), string(library.Season), string(library.Episode)
	seasons, err := b.media.List(ctx, library.ListMediaItemsFilter{ParentID: &seriesID, Type: &seasonType, Limit: 50})
	if err != nil {
		return nil
	}
	for _, s := range seasons {
		if s.IndexNumber == nil || *s.IndexNumber != 1 {
			continue
		}
		seasonID := string(s.ID)
		eps, err := b.media.List(ctx, library.ListMediaItemsFilter{ParentID: &seasonID, Type: &episodeType, Limit: 500})
		if err != nil {
			return nil
		}
		sort.Slice(eps, func(i, j int) bool {
			if eps[i].IndexNumber == nil || eps[j].IndexNumber == nil {
				return eps[i].IndexNumber != nil
			}
			return *eps[i].IndexNumber < *eps[j].IndexNumber
		})
		return eps
	}
	return nil
}
