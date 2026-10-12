---
source_sha: a960630483d1b2a05980e34cb7d0eacb371e9de2acf7fcd4af72e9609a09a8cf
reviewed: true
---
# API de plugins

Los endpoints en torno a los plugins: instalarlos y gestionarlos
(administrador), los webhooks (administrador) y las preferencias que la
interfaz de un plugin guarda para cada usuario. Escribir un plugin es otro
contrato, por gRPC sobre un socket local: consulta
[Escribir un plugin](../../plugins/dev-setup.md) y la
[Referencia de la API de plugins](../../plugins/api-reference.md).

| Especificación | Descripción |
|------|-------------|
| [admin_plugin.swagger.json](admin_plugin.swagger.json) | Instalar, activar, desactivar y desinstalar plugins; configuración, registros, límites de recursos, runtimes; vista previa y aplicación de una actualización; `IssuePluginToken` |
| [webhook.swagger.json](webhook.swagger.json) | Crear, listar, modificar y borrar webhooks; historial de envíos |
| [plugin_prefs.swagger.json](plugin_prefs.swagger.json) | Las preferencias por usuario de la interfaz de un plugin (el permiso `prefs`) |

Subir un paquete es un `POST /api/v1/admin/plugins/upload` multipart
(campos `plugin`, `sig` opcional, `accept_permission_changes`); `POST
/api/v1/admin/plugins/preview` lo analiza sin instalarlo. Las dos son rutas
de administrador.

## Autenticación

La gestión de plugins y los webhooks necesitan un administrador, como la
[API de administración](../admin/README.md). `plugin_prefs` es el único
servicio pensado para un token de plugin: la página anfitriona lo emite para
un plugin y un usuario, y el servicio solo responde con las filas de esa
pareja ([API del servidor](../README.md#authentication)).

<!-- sources
methods: proto/evey/v1/admin_plugin.proto, webhook.proto, plugin_prefs.proto
upload and preview routes: internal/adapters/driving/http/plugin_asset_handler.go
plugin token on prefs: internal/adapters/driving/grpcapi/interceptors/rbac.go - PluginMethodScopes ; internal/adapters/driving/grpcapi/plugin_prefs_handler.go
-->
