package pluginrunner

var KnownCapabilities = map[string]bool{
	"metadata.provider":   true,
	"media.read":          true,
	"media.write":         true,
	"webhook.receiver":    true,
	"notification.sender": true,
}

var KnownPermissions = map[string]bool{
	"network":             true,
	"filesystem":          true,
	"media.read":          true,
	"media.write":         true,
	"api.library":         true,
	"api.user":            true,
	"api.metadata":        true,
	"notification.sender": true,
	"subprocess":          true,
	// native-binary lets a plugin bundle compiled blobs / archives that
	// the static analyzer can't inspect. Opt-in only: the installer
	// blocks the install unless this permission is declared in the
	// manifest so the admin sees and accepts the elevated risk.
	"native-binary": true,
	// hardware.gpu grants the plugin the host's DRM render node
	// (/dev/dri/renderD12N) so it can run GPU compute (AI/ML, VAAPI). It is
	// the HIGHEST-risk permission: a render fd is a direct line into the
	// kernel GPU driver (ioctl attack surface) and a *per-GPU* — not
	// per-tenant — memory boundary shared with Evey's own transcoder. Admin-
	// only, deny-by-default, first-party/trusted plugins only. The grant is
	// the render node ALONE (never card0/dxg/nvidia*/kfd). See
	// docs/plugins.md + [[gpu-plugin-security]] for the residual risk the
	// install consent must surface.
	"hardware.gpu": true,
	// public: the iframe may read the non-sensitive media projection
	// (title/poster/year — never file paths). prefs: the iframe may read/write
	// the current user's preferences in this plugin's namespace. Both are
	// enforced deny-by-default on the scoped plugin token (see
	// internal/pkg/auth.AllowedPluginScopes + rbac PluginMethodScopes).
	"public": true,
	"prefs":  true,
}
