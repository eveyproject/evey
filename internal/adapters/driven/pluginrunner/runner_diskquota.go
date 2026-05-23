package pluginrunner

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/evey-project/evey/internal/domain/plugin"
)

// diskQuotaCheckInterval is how often a plugin's install directory is measured.
const diskQuotaCheckInterval = 30 * time.Second

// watchDiskQuota periodically measures the plugin's install directory and stops
// the plugin if it grows past the configured cap. cgroups bound CPU/RAM/PIDs
// but NOT disk space, so this is the disk guard: a plugin filling /data would
// otherwise DoS the server and sibling plugins. No-op when the quota is unset.
// It returns when the plugin exits (entry.done) or when it has stopped a plugin
// for breaching the quota.
func (r *Runner) watchDiskQuota(entry *processEntry, p *plugin.Plugin) {
	if r.diskMaxBytes <= 0 || p.InstallDir == "" {
		return
	}
	ticker := time.NewTicker(diskQuotaCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-entry.done:
			return
		case <-ticker.C:
			size, err := dirSize(p.InstallDir)
			if err != nil {
				continue // transient (dir mid-write); re-check next tick
			}
			if size > r.diskMaxBytes {
				slog.Warn("plugin exceeded disk quota, stopping it",
					slog.String("plugin", p.ID),
					slog.Int64("used_mb", size>>20),
					slog.Int64("max_mb", r.diskMaxBytes>>20))
				r.publishPluginError(context.Background(), p.ID,
					fmt.Sprintf("disk quota exceeded (%d MB > %d MB); plugin stopped", size>>20, r.diskMaxBytes>>20))
				_ = r.Stop(context.Background(), p.ID)
				return
			}
		}
	}
}

// dirSize returns the total size of regular files under dir. Unreadable entries
// are skipped so a transient permission/IO error never aborts the measurement.
func dirSize(dir string) (int64, error) {
	var total int64
	err := filepath.Walk(dir, func(_ string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if info != nil && info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}
