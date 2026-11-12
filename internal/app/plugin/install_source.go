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

// resolveInstallSource turns an install source into something downloadIfURL
// fetches, and names the repository the plugin can follow:
//   - a repository page (https://github.com/<owner>/<repo>) becomes the zip of
//     its latest release, pinned to the release's sha256;
//   - a release download link of that repository is kept as is;
//   - anything else (another URL, a local path) is kept and follows nothing.
func (s *InstallerService) resolveInstallSource(ctx context.Context, source string) (resolved, repo string, err error) {
	raw, _, _ := strings.Cut(source, "#")
	repo = repoOf(raw)
	if repo == "" {
		return source, "", nil
	}
	if !isRepositoryPage(raw) {
		return source, repo, nil
	}
	if s.releases == nil {
		return "", "", ErrNoReleaseSource
	}
	rel, err := s.releases.Latest(ctx, repo)
	if err != nil {
		return "", "", fmt.Errorf("resolve plugin source: %w: %w", ErrSourceUnavailable, err)
	}
	resolved = rel.ZipURL
	if rel.SHA256 != "" {
		resolved += "#sha256:" + rel.SHA256
	}
	return resolved, repo, nil
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
func (s *InstallerService) followRepository(ctx context.Context, p *plugin.Plugin, repo string) {
	if repo == "" || p == nil {
		return
	}
	stored, err := s.plugins.GetByID(ctx, p.ID)
	if err != nil || stored.UpdateSource != "" {
		return
	}
	if err := s.plugins.SetUpdateFollow(ctx, p.ID, repo, stored.AutoUpdate); err != nil {
		slog.WarnContext(ctx, "plugin: record the repository it follows", slog.String("plugin_id", p.ID), slog.Any("err", err))
		return
	}
	p.UpdateSource, p.AutoUpdate = repo, stored.AutoUpdate
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
	if _, err := s.plugins.GetByID(ctx, pluginID); err != nil {
		return nil, ErrPluginNotFound
	}
	if err := s.plugins.SetUpdateFollow(ctx, pluginID, repo, autoUpdate); err != nil {
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
