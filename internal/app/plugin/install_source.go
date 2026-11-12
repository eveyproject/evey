package plugin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/eveyproject/evey/internal/domain/plugin"
)

// ErrNoReleaseSource: a repository link was given but the server has no way
// to read releases (not wired).
var ErrNoReleaseSource = errors.New("repository links are not supported by this server")

// ErrRepositoryLinkPinned: a repository link carries a #sha256 pin, which
// cannot hold for "the latest release".
var ErrRepositoryLinkPinned = errors.New("a repository link installs its latest release and takes no #sha256 pin: pin the release's zip link instead")

// resolveInstallSource turns an install source into something downloadIfURL
// fetches, and names the repository the plugin can follow:
//   - a repository page (https://github.com/<owner>/<repo>) becomes the zip of
//     its latest release, pinned to the release's sha256;
//   - a release download link of that repository is kept as is;
//   - anything else (another URL, a local path) is kept and follows nothing.
func (s *InstallerService) resolveInstallSource(ctx context.Context, source string) (resolved string, follow repoRef, err error) {
	raw, fragment, _ := strings.Cut(source, "#")
	repo := repoOf(raw)
	if repo == "" {
		return source, repoRef{}, nil
	}
	if !isRepositoryPage(raw) {
		return source, repoRef{link: repo}, nil
	}
	if fragment != "" {
		// A pin on a link that means "whatever is latest" would be silently
		// dropped; refuse it rather than install something else than asked.
		return "", repoRef{}, ErrRepositoryLinkPinned
	}
	if s.releases == nil {
		return "", repoRef{}, ErrNoReleaseSource
	}
	id, canonical, err := s.releases.Identify(ctx, repo)
	if err != nil {
		return "", repoRef{}, fmt.Errorf("resolve plugin source: %w: %w", ErrSourceUnavailable, err)
	}
	rel, err := s.releases.Latest(ctx, id)
	if err != nil {
		return "", repoRef{}, fmt.Errorf("resolve plugin source: %w: %w", ErrSourceUnavailable, err)
	}
	resolved = rel.ZipURL
	if rel.SHA256 != "" {
		resolved += "#sha256:" + rel.SHA256
	}
	return resolved, repoRef{link: canonical, id: id}, nil
}

// repoRef is the repository an install source belongs to: its link, and its
// permanent id when it is already known (0 otherwise).
type repoRef struct {
	link string
	id   int64
}

// repoOf is the repository a link belongs to, empty when it is no repository
// link (another URL, a local path).
func repoOf(raw string) string {
	repo, err := plugin.NormalizeRepoURL(raw)
	if err != nil {
		return ""
	}
	return repo
}

// isRepositoryPage: the link names the repository itself, not a file in it.
func isRepositoryPage(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	return len(parts) == 2
}

// followRepository records the repository a plugin was installed from, the
// first time only: a plugin that already follows one, or that an admin set to
// follow another, keeps its choice.
func (s *InstallerService) followRepository(ctx context.Context, p *plugin.Plugin, ref repoRef) {
	if ref.link == "" || p == nil || s.releases == nil {
		return
	}
	stored, err := s.plugins.GetByID(ctx, p.ID)
	if err != nil || stored.UpdateSource != "" {
		return
	}
	repo, id := ref.link, ref.id
	if id == 0 {
		// Followed by its permanent id: a repository whose number is unknown
		// is not followed at all rather than followed by a name.
		id, repo, err = s.releases.Identify(ctx, ref.link)
		if err != nil {
			slog.WarnContext(ctx, "plugin: identify the repository it came from, not followed",
				slog.String("plugin_id", p.ID), slog.String("repo", ref.link), slog.Any("err", err))
			return
		}
	}
	if err := s.plugins.SetUpdateFollow(ctx, p.ID, repo, id, stored.AutoUpdate); err != nil {
		slog.WarnContext(ctx, "plugin: record the repository it follows", slog.String("plugin_id", p.ID), slog.Any("err", err))
		return
	}
	p.UpdateSource, p.UpdateSourceID, p.AutoUpdate = repo, id, stored.AutoUpdate
}

// settleUpdateState: once a plugin runs the release its last check reported
// (an admin approved a held one, or installed it by hand), that release no
// longer waits, so the state stops saying it does.
func (s *InstallerService) settleUpdateState(ctx context.Context, p *plugin.Plugin) {
	st := p.UpdateState
	if st.LatestVersion == "" || st.Outcome == plugin.UpdateUpToDate || st.Outcome == plugin.UpdateApplied {
		return
	}
	if CompareSemver(st.LatestVersion, p.Version) > 0 {
		return
	}
	st.Outcome, st.Reason = plugin.UpdateUpToDate, ""
	if err := s.plugins.SetUpdateState(ctx, p.ID, st); err != nil {
		slog.WarnContext(ctx, "plugin: settle the update state", slog.String("plugin_id", p.ID), slog.Any("err", err))
		return
	}
	p.UpdateState = st
}

// SetUpdateFollow is the admin's choice for one plugin: the repository it
// follows (empty: none) and whether a release installs without them.
func (s *InstallerService) SetUpdateFollow(ctx context.Context, pluginID, source string, autoUpdate bool) (*plugin.Plugin, error) {
	repo, err := plugin.NormalizeRepoURL(source)
	if err != nil {
		return nil, err
	}
	stored, err := s.plugins.GetByID(ctx, pluginID)
	if err != nil {
		return nil, ErrPluginNotFound
	}
	var id int64
	switch {
	case repo == "":
	case repo == stored.UpdateSource && stored.UpdateSourceID > 0:
		id = stored.UpdateSourceID
	case s.releases == nil:
		return nil, ErrNoReleaseSource
	default:
		// A new repository is identified now: it is followed by that number.
		if id, repo, err = s.releases.Identify(ctx, repo); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrSourceUnavailable, err)
		}
	}
	if err := s.plugins.SetUpdateFollow(ctx, pluginID, repo, id, autoUpdate); err != nil {
		return nil, err
	}
	if repo == "" {
		// Following nothing leaves nothing to report about.
		if err := s.plugins.SetUpdateState(ctx, pluginID, plugin.UpdateState{}); err != nil {
			return nil, err
		}
	}
	return s.plugins.GetByID(ctx, pluginID)
}
