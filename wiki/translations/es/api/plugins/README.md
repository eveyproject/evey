---
source_sha: 7d46731b1dfd44908d653fde2e55780a150fa9681de75fa6518859def0fe4dbd
---
# API de extensiones

Endpoints para gestionar extensiones e integraciones por webhook.

## Especificaciones

| Especificación | Descripción |
|------|-------------|
| [admin_plugin.swagger.json](admin_plugin.swagger.json) | Instalar, activar, desactivar y desinstalar extensiones. Configuración y logs de las extensiones. Entornos de ejecución. |
| [webhook.swagger.json](webhook.swagger.json) | Crear, leer, modificar y borrar webhooks; historial de entregas |

## Desarrollo de extensiones

Las extensiones se comunican con el servidor Evey por gRPC. Vea la [documentación de extensiones](../../plugins.md) para la guía completa de desarrollo, que incluye:
- Ciclo de vida de una extensión (instalar, activar, configurar, desactivar, desinstalar)
- Hooks de eventos disponibles
- Esquema de configuración
- Aislamiento en entorno protegido (seccomp, aislamiento de procesos)
