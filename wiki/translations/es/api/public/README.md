---
source_sha: d5c0534aa9aa7e7c333f76023148a199fc8b6bf2f16cee4f6c80face79daf6b2
reviewed: true
---
# API pública

Los endpoints a los que puede llamar cualquier usuario conectado: los que usan
las aplicaciones para explorar una biblioteca, reproducir un título, guardar
favoritos y listas de reproducción, y pedir un medio. Un archivo por servicio.

| Especificación | Descripción |
|------|-------------|
| [library.swagger.json](library.swagger.json) | Bibliotecas, elementos multimedia, búsqueda, personas; la proyección `public` que puede leer la interfaz de un plugin |
| [playback.swagger.json](playback.swagger.json) | Streaming, seguimiento del progreso, historial de visionado, reanudación |
| [user.swagger.json](user.swagger.json) | Autenticación, perfiles, claves de API, Quick Connect |
| [collection.swagger.json](collection.swagger.json) | Colecciones inteligentes y manuales |
| [favorite.swagger.json](favorite.swagger.json) | Elementos favoritos |
| [feed.swagger.json](feed.swagger.json) | Secciones del inicio (seguir viendo, añadidos recientemente...) |
| [playlist.swagger.json](playlist.swagger.json) | Listas de reproducción, hechas a mano o por reglas |
| [rating.swagger.json](rating.swagger.json) | Valoraciones de los usuarios |
| [share.swagger.json](share.swagger.json) | Enlaces públicos para compartir |
| [syncplay.swagger.json](syncplay.swagger.json) | Salas de reproducción sincronizada |
| [notification.swagger.json](notification.swagger.json) | Notificaciones dentro de la aplicación |
| [push.swagger.json](push.swagger.json) | Suscripciones a notificaciones push web |
| [music.swagger.json](music.swagger.json) | Biblioteca de música (artistas, álbumes, pistas) |
| [photo.swagger.json](photo.swagger.json) | Biblioteca de fotos (álbumes, fotos) |
| [media_request.swagger.json](media_request.swagger.json) | Solicitudes de medios: búsqueda, envío, política; las respuestas del administrador y el acceso a las solicitudes |

## Autenticación

Los endpoints aceptan `Authorization: Bearer <token>`: un token de sesión
obtenido al iniciar sesión, o una clave de API dentro de sus alcances
([API del servidor](../README.md#authentication)).
Abiertos sin credencial: iniciar sesión y renovar una sesión, registrarse con
una invitación, Quick Connect, restablecer la contraseña, el asistente de
configuración y su estado, y los enlaces públicos de descarga.

<!-- sources
auth: internal/adapters/driving/grpcapi/interceptors/auth.go ; public methods: internal/adapters/driving/grpcapi/interceptors/rbac.go - PublicMethods
public projection: proto/evey/v1/library.proto - GetPublicMediaItem, ListPublicMediaItems
-->
