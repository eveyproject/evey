---
source_sha: cc4cecd7aee5e32086fae7de125434b98b804bf28bed8ea336efed0bb38c02bb
reviewed: true
---
# API de administración

Los endpoints reservados a los administradores: a los que llama
**Administración** en la aplicación web. Un archivo por servicio, agrupados
por área.

### Sistema
| Especificación | Descripción |
|------|-------------|
| [admin_system.swagger.json](admin_system.swagger.json) | Panel, información del sistema, asistente de configuración |
| [server_config.swagger.json](server_config.swagger.json) | Almacén clave/valor de la configuración del servidor |
| [remote_access.swagger.json](remote_access.swagger.json) | Acceso remoto y ajustes de red |
| [backup.swagger.json](backup.swagger.json) | Copia de seguridad y restauración de la base de datos |
| [backup_schedule.swagger.json](backup_schedule.swagger.json) | Copias programadas: cron, retención, ejecutar ahora |
| [activity.swagger.json](activity.swagger.json) | Historial de reproducción, historial de inicios de sesión, métricas del servidor |
| [admin_session.swagger.json](admin_session.swagger.json) | Registro de auditoría |
| [admin_task.swagger.json](admin_task.swagger.json) | Tareas programadas en segundo plano |
| [presence.swagger.json](presence.swagger.json) | Quién está conectado ahora mismo (el latido de las aplicaciones y la lista de administración) |

### Medios y transcodificación
| Especificación | Descripción |
|------|-------------|
| [admin_media.swagger.json](admin_media.swagger.json) | Salud de medios (escaneos, reparaciones, historial y deshacer), borrar elementos multimedia, actualizar metadatos, generar el trickplay |
| [metadata.swagger.json](metadata.swagger.json) | Proveedores de metadatos, búsqueda, correcciones manuales |
| [transcoding.swagger.json](transcoding.swagger.json) | Perfiles de transcodificación, detección del hardware, trabajos activos |
| [transcode_cache.swagger.json](transcode_cache.swagger.json) | Gestión de la caché de transcodificación |
| [mkv.swagger.json](mkv.swagger.json) | Herramientas para el contenedor MKV (pistas, capítulos, mux/demux) |
| [admin_intro.swagger.json](admin_intro.swagger.json) | Caché de tiempos de intro: invalidar un episodio, rellenar lo que falta |

### Usuarios y políticas
| Especificación | Descripción |
|------|-------------|
| [library_access.swagger.json](library_access.swagger.json) | Control de acceso a las bibliotecas por usuario |
| [parental.swagger.json](parental.swagger.json) | Control parental y PIN |
| [playback_settings.swagger.json](playback_settings.swagger.json) | Ajustes globales de reproducción |

## Autenticación

Cada endpoint necesita un administrador: el token de sesión de una cuenta de
administrador, o una clave de API, que siempre pertenece a uno y debe llevar
el alcance que pide el método (una clave sin alcances no llega a ningún
método de administración). Aquí se rechaza un token de plugin, tenga los
alcances que tenga ([API del servidor](../README.md#authentication)).

<!-- sources
admin methods: internal/adapters/driving/grpcapi/interceptors/rbac.go - MethodRoles ; api key scope on admin methods: interceptors/auth.go - authorize ; plugin token floor: auth.go - authorizePluginToken
-->
