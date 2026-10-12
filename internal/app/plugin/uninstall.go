package plugin

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// uninstall.go holds plugin removal: the guaranteed-clean UninstallPlugin
// (stop → kill → wipe dir → delete row) and the startup CleanupOrphans sweep
// that reaps plg-* dirs with no matching DB row.

// UninstallPlugin guarantees a clean removal: any process is stopped (and
// SIGKILL'd if Stop hangs), the install dir is removed (with retries to ride
// out busy-file races during sandbox teardown), the DB row is deleted, and
// permissions/event hooks fire. The admin never sees "uninstall failed"
// because the runner refused to die or a file was momentarily locked -
// those steps are best-effort. Deleting the DB row is not: if it fails the
// plugin is still considered installed, so the error is returned and the
// PluginUninstalled event is not published.
func (s *InstallerService) UninstallPlugin(ctx context.Context, id string) error {
	ctx, span := tracing.StartSpan(ctx, "app.plugin.UninstallPlugin")
	defer span.End()
	p, err := s.plugins.GetByID(ctx, id)
	if err != nil {
		return ErrPluginNotFound
	}

	// 1) Stop runner gracefully with a tight deadline. If it hangs or errors,
	//    SIGKILL the whole process group so any child (the plugin process and
	//    its bwrap sandbox) dies with it. IsStarted (not just Running) so a
	//    plugin still in Initializing - or stuck there because it never finished
	//    the IPC handshake - is terminated too, instead of having its row and
	//    dir wiped while its process runs on until the next server restart.
	if p.Status.IsStarted() {
		stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		stopErr := s.runner.Stop(stopCtx, id)
		cancel()
		if stopErr != nil {
			slog.WarnContext(ctx, "plugin Stop failed, falling back to Kill",
				slog.String("pluginID", id), slog.Any("err", stopErr))
			if killErr := s.runner.Kill(id); killErr != nil {
				slog.WarnContext(ctx, "plugin Kill failed (will continue)",
					slog.String("pluginID", id), slog.Any("err", killErr))
			}
			// Give the kernel a beat to recycle any FDs the plugin held open
			// (sandbox mount, log files, …) before we attempt RemoveAll.
			time.Sleep(500 * time.Millisecond)
		}
	}

	// 2) Drop cached permissions before we hand the FS path to RemoveAll -
	//    permChecker is best-effort and never blocks uninstall.
	if s.permChecker != nil {
		s.permChecker.InvalidatePermissions(p.ID)
	}

	// 3) Wipe the install dir, retrying on transient busy-file errors that
	//    surface when the sandbox isn't fully torn down yet. Three attempts
	//    with exponential backoff (100ms, 500ms, 2s) covers the typical
	//    bwrap unmount window. If it still fails we log and keep going so
	//    the DB row gets cleaned anyway - a future CleanupOrphans pass will
	//    eventually pick up a stuck dir.
	if p.InstallDir != "" && s.fs != nil {
		backoffs := []time.Duration{100 * time.Millisecond, 500 * time.Millisecond, 2 * time.Second}
		var lastErr error
		for i, wait := range backoffs {
			if rmErr := s.fs.RemoveAll(p.InstallDir); rmErr == nil {
				lastErr = nil
				break
			} else {
				lastErr = rmErr
				if i < len(backoffs)-1 {
					time.Sleep(wait)
				}
			}
		}
		if lastErr != nil {
			slog.ErrorContext(ctx, "plugin install dir not removed after retries (will be picked up by CleanupOrphans)",
				slog.String("pluginID", id), slog.String("dir", p.InstallDir), slog.Any("err", lastErr))
		}
	}

	// 4) Delete the DB row. If this fails the FS is already gone (or queued
	//    for cleanup) but the plugin isn't actually uninstalled yet - the row
	//    still exists, so we must surface the error instead of reporting
	//    success, and skip the PluginUninstalled event below (nothing was
	//    actually uninstalled). The admin can retry; a repeated attempt is
	//    safe since steps 1-3 are idempotent.
	if err := s.plugins.Delete(ctx, id); err != nil {
		slog.ErrorContext(ctx, "plugin DB delete failed (admin can retry)",
			slog.String("pluginID", id), slog.Any("err", err))
		return fmt.Errorf("uninstall %s: delete plugin row: %w", id, err)
	}

	// 5) Fire the event so subscribers (slot registry, theme cache, …) drop
	//    their references. Best-effort.
	evtUninstalled := plugin.PluginUninstalled{PluginID: id, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtUninstalled); err != nil {
		slog.WarnContext(ctx, "publish event", slog.String("event", evtUninstalled.EventName()), slog.Any("err", err))
	}
	return nil
}

// CleanupOrphans walks the configured pluginDir and removes any plg-*
// directory that has no matching row in the plugin repository. Designed to
// be called once at server startup so historical leftovers (uninstalls that
// crashed before the FS removal succeeded) get reaped automatically.
// Best-effort: errors are logged, never returned. Returns the number of
// dirs that were removed.
func (s *InstallerService) CleanupOrphans(ctx context.Context) int {
	ctx, span := tracing.StartSpan(ctx, "app.plugin.CleanupOrphans")
	defer span.End()
	if s.fs == nil || s.pluginDir == "" {
		return 0
	}
	listDirs := s.listDirsFn
	if listDirs == nil {
		listDirs = defaultListDirs
	}
	names, err := listDirs(s.pluginDir)
	if err != nil {
		slog.WarnContext(ctx, "cleanup orphans: read plugin dir", slog.String("dir", s.pluginDir), slog.Any("err", err))
		return 0
	}

	all, err := s.plugins.List(ctx)
	if err != nil {
		slog.WarnContext(ctx, "cleanup orphans: list plugins from db", slog.Any("err", err))
		return 0
	}
	known := make(map[string]struct{}, len(all))
	for _, p := range all {
		known[p.ID] = struct{}{}
	}

	removed := 0
	for _, name := range names {
		if len(name) < 4 || name[:4] != "plg-" {
			continue
		}
		if _, ok := known[name]; ok {
			continue
		}
		full := filepath.Join(s.pluginDir, name)
		if err := s.fs.RemoveAll(full); err != nil {
			slog.WarnContext(ctx, "cleanup orphans: remove dir failed",
				slog.String("dir", full), slog.Any("err", err))
			continue
		}
		slog.InfoContext(ctx, "cleanup orphans: removed orphan plugin dir", slog.String("dir", full))
		removed++
	}
	if removed > 0 {
		slog.InfoContext(ctx, "cleanup orphans: complete", slog.Int("removed", removed))
	}
	return removed
}
