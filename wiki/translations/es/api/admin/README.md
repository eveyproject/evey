---
source_sha: f1907adff4cc0c2d70169aaefdc143b3040b23f0c75f5fc0813c852b00f5c66f
reviewed: false
---
# API de administración

Endpoints de administración reservados a los usuarios con privilegios de administrador.

## Especificaciones

### Sistema
| Especificación | Descripción |
|------|-------------|
| [admin_system.swagger.json](admin_system.swagger.json) | Panel, información del sistema, asistente de configuración |
| [server_config.swagger.json](server_config.swagger.json) | Almacén clave/valor de la configuración del servidor |
| [remote_access.swagger.json](remote_access.swagger.json) | Acceso remoto y ajustes de red |
| [backup.swagger.json](backup.swagger.json) | Copia de seguridad y restauración de la base de datos |
| [backup_schedule.swagger.json](backup_schedule.swagger.json) | Copias programadas: cron, retención, ejecutar ahora |
| [activity.swagger.json](activity.swagger.json) | Historial de reproducciones, historial de inicios de sesión, métricas del servidor |
| [admin_session.swagger.json](admin_session.swagger.json) | Registro de auditoría |
| [admin_task.swagger.json](admin_task.swagger.json) | Tareas programadas en segundo plano |
| [presence.swagger.json](presence.swagger.json) | Quién está conectado ahora mismo (el latido de las aplicaciones y la lista de administración) |

### Medios y transcodificación
| Especificación | Descripción |
|------|-------------|
| [admin_media.swagger.json](admin_media.swagger.json) | Salud de medios (análisis, reparaciones, historial y deshacer), eliminar medios, actualizar metadatos, generar trickplay |
| [metadata.swagger.json](metadata.swagger.json) | Proveedores de metadatos, búsqueda, anulaciones |
| [transcoding.swagger.json](transcoding.swagger.json) | Perfiles de transcodificación, detección de hardware, tareas activas |
| [transcode_cache.swagger.json](transcode_cache.swagger.json) | Gestión de la caché de transcodificación |
| [mkv.swagger.json](mkv.swagger.json) | Herramientas del contenedor MKV (pistas, capítulos, mux/demux) |
| [admin_intro.swagger.json](admin_intro.swagger.json) | Caché de tiempos de intro: invalidar un episodio, relleno |

### Usuarios y políticas
| Especificación | Descripción |
|------|-------------|
| [library_access.swagger.json](library_access.swagger.json) | Control de acceso a las bibliotecas por usuario |
| [parental.swagger.json](parental.swagger.json) | Control parental y PIN |
| [playback_settings.swagger.json](playback_settings.swagger.json) | Ajustes globales de reproducción |

## Autenticación

Cada endpoint exige un administrador: el token de sesión de una cuenta de administrador, o una clave de API, que siempre pertenece a uno (sus scopes pueden restringirla).
