---
source_sha: d950d41e450f1d05a29cf775d0b79159572cfa1353c7e02ac009671bb0bcefc1
reviewed: false
---
# API pública

Endpoints para los usuarios, disponibles para todos los usuarios autenticados.

## Especificaciones

| Especificación | Descripción |
|------|-------------|
| [library.swagger.json](library.swagger.json) | Bibliotecas, medios, búsqueda, personas |
| [playback.swagger.json](playback.swagger.json) | Streaming, seguimiento del progreso, historial, reanudación |
| [user.swagger.json](user.swagger.json) | Autenticación, perfiles, claves de API, Quick Connect |
| [collection.swagger.json](collection.swagger.json) | Colecciones inteligentes y manuales |
| [favorite.swagger.json](favorite.swagger.json) | Favoritos |
| [feed.swagger.json](feed.swagger.json) | Secciones de la página de inicio (seguir viendo, añadidos recientemente, etc.) |
| [playlist.swagger.json](playlist.swagger.json) | Listas de reproducción (crear, reordenar, compartir) |
| [rating.swagger.json](rating.swagger.json) | Valoraciones de los usuarios |
| [share.swagger.json](share.swagger.json) | Enlaces públicos para compartir |
| [syncplay.swagger.json](syncplay.swagger.json) | Salas de reproducción sincronizada |
| [notification.swagger.json](notification.swagger.json) | Notificaciones en la aplicación |
| [push.swagger.json](push.swagger.json) | Suscripciones de push web |
| [music.swagger.json](music.swagger.json) | Biblioteca de música (artistas, álbumes, pistas) |
| [photo.swagger.json](photo.swagger.json) | Biblioteca de fotos (álbumes, fotos) |
| [media_request.swagger.json](media_request.swagger.json) | Solicitudes de contenido: búsqueda, envío, política; las respuestas del administrador y el acceso a las solicitudes |

## Autenticación

Todos los endpoints exigen un token JWT válido mediante `Authorization: Bearer <token>`, salvo `/api/v1/auth/login` y `/api/v1/setup/*`.
