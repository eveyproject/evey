package plugin

import (
	"errors"
	"log/slog"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/plugin"
)

var ErrPermissionDenied = errors.New("plugin permission denied")

// PermissionChecker verifies that plugins have the required permissions before
// executing HostService RPCs. Permissions are cached in memory and invalidated
// on install/uninstall/update.
type PermissionChecker struct {
	mu    sync.RWMutex
	cache map[string]map[string]bool // pluginID -> permission -> true
	repo  plugin.PluginRepository
}

func NewPermissionChecker(repo plugin.PluginRepository) *PermissionChecker {
	return &PermissionChecker{
		cache: make(map[string]map[string]bool),
		repo:  repo,
	}
}

// CheckPermission returns nil if the plugin has requiredPerm, ErrPermissionDenied otherwise.
func (pc *PermissionChecker) CheckPermission(pluginID, requiredPerm string) error {
	pc.mu.RLock()
	perms, ok := pc.cache[pluginID]
	pc.mu.RUnlock()

	if ok {
		if perms[requiredPerm] {
			return nil
		}
		slog.Warn("plugin permission denied",
			slog.String("plugin_id", pluginID),
			slog.String("permission", requiredPerm))
		return ErrPermissionDenied
	}

	return ErrPermissionDenied
}

// CheckAnyPermission returns nil if the plugin has at least one of the given permissions.
func (pc *PermissionChecker) CheckAnyPermission(pluginID string, perms ...string) error {
	pc.mu.RLock()
	cached, ok := pc.cache[pluginID]
	pc.mu.RUnlock()

	if !ok {
		return ErrPermissionDenied
	}

	for _, p := range perms {
		if cached[p] {
			return nil
		}
	}

	slog.Warn("plugin permission denied",
		slog.String("plugin_id", pluginID),
		slog.String("required_any", perms[0]))
	return ErrPermissionDenied
}

// LoadPermissions caches permissions for a plugin.
func (pc *PermissionChecker) LoadPermissions(pluginID string, permissions []string) {
	m := make(map[string]bool, len(permissions))
	for _, p := range permissions {
		m[p] = true
	}
	pc.mu.Lock()
	pc.cache[pluginID] = m
	pc.mu.Unlock()
}

// InvalidatePermissions removes cached permissions for a plugin.
func (pc *PermissionChecker) InvalidatePermissions(pluginID string) {
	pc.mu.Lock()
	delete(pc.cache, pluginID)
	pc.mu.Unlock()
}
