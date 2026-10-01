---
source_sha: eea62b3dd7b2be9c176a9b8431d6ee58738ecf22c64c0f154bb1b1211878f293
reviewed: false
---
# API de plugins

Endpoints para gestionar los plugins y las integraciones por webhook.

## Especificaciones

| Especificación | Descripción |
|------|-------------|
| [admin_plugin.swagger.json](admin_plugin.swagger.json) | Instalar, activar, desactivar y desinstalar plugins. Configuración y registros de los plugins. Entornos de ejecución de plugins. |
| [webhook.swagger.json](webhook.swagger.json) | CRUD de webhooks, historial de entregas |
| [plugin_prefs.swagger.json](plugin_prefs.swagger.json) | Las preferencias por usuario de la interfaz de un plugin (el permiso `prefs`) |

## Desarrollo de plugins

Los plugins se comunican con el servidor Evey por gRPC. Consulta la [documentación de plugins](../../plugins.md) para la guía de desarrollo completa, que incluye:
- El ciclo de vida de un plugin (instalar, activar, configurar, desactivar, desinstalar)
- Los ganchos de eventos disponibles
- El esquema de configuración
- El sandbox de seguridad (seccomp, aislamiento de procesos)
