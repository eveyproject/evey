---
source_sha: e314c511a93178cdc35b47b8bcd886cb60fd2ea303b7fc7d6f6c3c9ca1f9b6d0
---
# API de administración

Endpoints de administración, reservados a los usuarios con privilegios de administrador.

## Especificaciones

### Sistema
| Especificación | Descripción |
|------|-------------|
| [admin_system.swagger.json](admin_system.swagger.json) | Panel, información del sistema, asistente de instalación |
| [server_config.swagger.json](server_config.swagger.json) | Almacén clave-valor de la configuración del servidor |
| [remote_access.swagger.json](remote_access.swagger.json) | Acceso remoto y ajustes de red |
| [backup.swagger.json](backup.swagger.json) | Copia de seguridad y restauración de la base de datos |
| [activity.swagger.json](activity.swagger.json) | Historial de reproducción, historial de inicios de sesión, métricas del servidor |
| [admin_session.swagger.json](admin_session.swagger.json) | Registro de auditoría |
| [admin_task.swagger.json](admin_task.swagger.json) | Tareas programadas en segundo plano |

### Medios y transcodificación
| Especificación | Descripción |
|------|-------------|
| [admin_media.swagger.json](admin_media.swagger.json) | Borrar elementos multimedia, actualizar metadatos, generar trickplay |
| [metadata.swagger.json](metadata.swagger.json) | Proveedores de metadatos, búsqueda, sobrescrituras |
| [transcoding.swagger.json](transcoding.swagger.json) | Perfiles de transcodificación, detección de hardware, trabajos activos |
| [transcode_cache.swagger.json](transcode_cache.swagger.json) | Gestión de la caché de transcodificación |
| [mkv.swagger.json](mkv.swagger.json) | Herramientas para contenedores MKV (pistas, capítulos, mux/demux) |

### Usuarios y políticas
| Especificación | Descripción |
|------|-------------|
| [library_access.swagger.json](library_access.swagger.json) | Control de acceso a bibliotecas por usuario |
| [parental.swagger.json](parental.swagger.json) | Control parental y PIN |
| [playback_settings.swagger.json](playback_settings.swagger.json) | Ajustes globales de reproducción |

## Autenticación

Todos los endpoints requieren un token JWT de administrador válido.
