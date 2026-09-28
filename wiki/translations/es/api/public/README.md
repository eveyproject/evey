---
source_sha: 4609c608404f7d7614f31c97a0e6a7ce865e34c4d562caf6ca04af7ced28096a
---
# API pública

Endpoints para los usuarios, disponibles para todos los usuarios autenticados.

## Especificaciones

| Especificación | Descripción |
|------|-------------|
| [library.swagger.json](library.swagger.json) | Bibliotecas, elementos multimedia, búsqueda, personas |
| [playback.swagger.json](playback.swagger.json) | Streaming, seguimiento del progreso, historial de visualización, reanudación |
| [user.swagger.json](user.swagger.json) | Autenticación, perfiles, claves API, conexión rápida |
| [collection.swagger.json](collection.swagger.json) | Colecciones inteligentes y manuales |
| [favorite.swagger.json](favorite.swagger.json) | Favoritos |
| [feed.swagger.json](feed.swagger.json) | Secciones de la página de inicio (seguir viendo, añadidos recientemente, etc.) |
| [filter.swagger.json](filter.swagger.json) | Filtros guardados |
| [playlist.swagger.json](playlist.swagger.json) | Listas de reproducción (crear, reordenar, compartir) |
| [rating.swagger.json](rating.swagger.json) | Valoraciones de los usuarios |
| [share.swagger.json](share.swagger.json) | Enlaces públicos para compartir |
| [syncplay.swagger.json](syncplay.swagger.json) | Salas de reproducción sincronizada |
| [notification.swagger.json](notification.swagger.json) | Notificaciones dentro de la aplicación |
| [push.swagger.json](push.swagger.json) | Suscripciones web push |
| [music.swagger.json](music.swagger.json) | Biblioteca de música (artistas, álbumes, pistas) |
| [photo.swagger.json](photo.swagger.json) | Biblioteca de fotos (álbumes, fotos) |

## Autenticación

Todos los endpoints requieren un token JWT válido mediante `Authorization: Bearer <token>`, salvo `/api/v1/auth/login` y `/api/v1/setup/*`.
