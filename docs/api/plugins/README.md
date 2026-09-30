# Plugin API

Endpoints for managing plugins and webhook integrations.

## Specs

| Spec | Description |
|------|-------------|
| [admin_plugin.swagger.json](admin_plugin.swagger.json) | Install, enable, disable, uninstall plugins. Plugin config and logs. Plugin runtimes. |
| [webhook.swagger.json](webhook.swagger.json) | CRUD webhooks, delivery history |
| [plugin_prefs.swagger.json](plugin_prefs.swagger.json) | A plugin interface's per-user preferences (the `prefs` permission) |

## Plugin Development

Plugins communicate with the Evey server via gRPC. See [Plugin Documentation](../../plugins.md) for the full development guide including:
- Plugin lifecycle (install, enable, configure, disable, uninstall)
- Available event hooks
- Configuration schema
- Security sandboxing (seccomp, process isolation)
