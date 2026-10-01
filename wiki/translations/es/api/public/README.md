---
source_sha: 58e220cb177619c27f3d55f0ae3d1c6a3b4df50ab6ace660df93aa8ccf6e6df9
reviewed: false
---
# API pública

Endpoints para los usuarios, disponibles para todos los usuarios autenticados.

## Especificaciones

| Especificación | Descripción |
|------|-------------|
| [library.swagger.json](library.swagger.json) | Bibliotecas, medios, búsqueda, personas |
| [playback.swagger.json](playback.swagger.json) | Streaming, seguimiento del progreso, historial, reanudación |
| [user.swagger.json](user.swagger.json) | Autenticación, perfiles, claves de API, Conexión rápida |
| [collection.swagger.json](collection.swagger.json) | Colecciones inteligentes y manuales |
| [favorite.swagger.json](favorite.swagger.json) | Favoritos |
| [feed.swagger.json](feed.swagger.json) | Secciones de la página de inicio (seguir viendo, añadidos recientemente, etc.) |
| [playlist.swagger.json](playlist.swagger.json) | Listas de reproducción, hechas a mano o por reglas |
| [rating.swagger.json](rating.swagger.json) | Valoraciones de los usuarios |
| [share.swagger.json](share.swagger.json) | Enlaces públicos para compartir |
| [syncplay.swagger.json](syncplay.swagger.json) | Salas de reproducción sincronizada |
| [notification.swagger.json](notification.swagger.json) | Notificaciones en la aplicación |
| [push.swagger.json](push.swagger.json) | Suscripciones de push web |
| [music.swagger.json](music.swagger.json) | Biblioteca de música (artistas, álbumes, pistas) |
| [photo.swagger.json](photo.swagger.json) | Biblioteca de fotos (álbumes, fotos) |
| [media_request.swagger.json](media_request.swagger.json) | Solicitudes de contenido: búsqueda, envío, política; las respuestas del administrador y el acceso a las solicitudes |

## Autenticación

Los endpoints reciben `Authorization: Bearer <token>`: un token de sesión obtenido al iniciar sesión, o una clave de API (ver [Webhooks y claves de API](../../webhooks-api-keys.md)). Accesibles sin él: iniciar y renovar sesión, registrarse con una invitación, Conexión rápida, restablecer la contraseña, el asistente de configuración y su estado, y los enlaces de descarga públicos.
