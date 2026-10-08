# Plugin API

The endpoints around plugins: installing and managing them (administrator),
webhooks (administrator), and the preferences a plugin's interface keeps per
user. Writing a plugin is a different contract, over gRPC on a local socket:
see [Writing a plugin](../../plugins/dev-setup.md) and the
[Plugin API reference](../../plugins/api-reference.md).

| Spec | Description |
|------|-------------|
| [admin_plugin.swagger.json](admin_plugin.swagger.json) | Install, enable, disable, uninstall plugins; config, logs, resource limits, runtimes; update preview and apply; `IssuePluginToken` |
| [webhook.swagger.json](webhook.swagger.json) | Create, list, update and delete webhooks; delivery history |
| [plugin_prefs.swagger.json](plugin_prefs.swagger.json) | A plugin interface's per-user preferences (the `prefs` permission) |

Uploading a package is a multipart `POST /api/v1/admin/plugins/upload`
(fields `plugin`, optional `sig`, `accept_permission_changes`); `POST
/api/v1/admin/plugins/preview` analyses without installing. Both are
administrator routes.

## Authentication

Plugin management and webhooks need an administrator, like the
[Admin API](../admin/README.md). `plugin_prefs` is the one service made for a
plugin token: the host page mints it for one plugin and one user, and the
service answers only that pair's rows ([Server API](../README.md#authentication)).

<!-- sources
methods: proto/evey/v1/admin_plugin.proto, webhook.proto, plugin_prefs.proto
upload and preview routes: internal/adapters/driving/http/plugin_asset_handler.go
plugin token on prefs: internal/adapters/driving/grpcapi/interceptors/rbac.go - PluginMethodScopes ; internal/adapters/driving/grpcapi/plugin_prefs_handler.go
-->
