---
source_sha: 632dddb3ad048221e832d8c1f63008289641932b655a32564877f6dcc316bb04
reviewed: true
---
# Política de privacidad

Última actualización: 2026-10-04.

Esta política se aplica a las apps de Evey para Android y Android TV.

En resumen: el proyecto Evey no tiene ningún servicio que reciba tus datos.
No hay cuenta con nosotros, ni estadísticas de uso, ni publicidad, ni
informes de errores que nos lleguen. Las apps se comunican con el servidor Evey
instalado y gestionado por ti o por la persona que te invitó.

## Qué envían las apps, y a quién

**A tu servidor Evey, y a nada más que gestionemos nosotros.** Las apps
envían lo que un reproductor multimedia necesita para funcionar:

- tu nombre de usuario y tu contraseña cuando inicias sesión (o el código de
  Conexión rápida), y después un token de sesión, una clave que mantiene tu
  sesión abierta;
- lo que exploras, buscas, reproduces, valoras, marcas como favorito o
  solicitas, y el punto de reproducción donde te quedaste, para que puedas
  seguir en otro dispositivo;
- el nombre del dispositivo (tal como lo da Android), para que puedas
  distinguir tus dispositivos en **Reproducir en** y en la lista de sesiones
  del servidor;
- informes técnicos de reproducción: qué formatos de audio y de vídeo puede
  decodificar el dispositivo, y los errores que surgen al reproducir, para
  que el servidor elija un flujo que el dispositivo pueda reproducir;
- una imagen, solo cuando eliges una como avatar de perfil, y los archivos
  que tú mismo eliges (un subtítulo externo, un plugin, una copia de
  seguridad) como administrador.

Quién puede leer estos datos lo decide quien gestiona ese servidor. El
proyecto Evey no tiene acceso a ellos.

**A terceros, solo en estos casos:**

- **Pósteres y portadas de álbumes.** Algunas imágenes que se muestran en las
  solicitudes y en la música vienen directamente de su fuente pública (The
  Movie Database, el Cover Art Archive, IMDb). El dispositivo las descarga
  como lo haría un navegador web, lo que da a conocer su dirección IP a ese
  sitio.
- **Envío a Chromecast.** Enviar a un Chromecast pasa por Google Cast, que
  forma parte de los servicios de Google Play de tu teléfono. El Chromecast
  carga la página receptora de Evey desde `cast.eveyproject.org`, alojada en
  GitHub Pages. El contenido en sí llega directamente desde tu servidor.
- **Búsqueda por voz en Android TV.** El botón del micrófono pasa el control
  al reconocimiento de voz del propio televisor (normalmente el de Google).
  La app recibe el texto reconocido, nunca la grabación.
- **Enlaces que abres.** Un tráiler, una página de The Movie Database o de
  IMDb, o la página de Play Store de una actualización se abren en la app o
  el navegador correspondiente, con la política de ese servicio.
- **Plugins.** Los plugins los instala el administrador del servidor. Un
  plugin puede mostrar contenido de un servicio externo (un plugin del
  tiempo, por ejemplo).

## Lo que se queda en el dispositivo

La dirección del servidor, tu token de sesión (en el almacenamiento cifrado
de Android), tus preferencias, y las películas y la música que descargas para
verlas o escucharlas sin conexión. Si desinstalas la app, se borran. El
desbloqueo biométrico (huella o cara), cuando lo activas, lo gestiona
Android: la app solo sabe si ha funcionado o no.

## Permisos

| Permiso | Para qué |
| --- | --- |
| Internet, estado de la Wi-Fi | Llegar a tu servidor, distinguir la red de tu casa de una red de fuera. |
| Cámara | Escanear un código QR de la Conexión rápida. No se graba ni se envía nada. |
| Notificaciones, servicio en primer plano | Mostrar el avance de la reproducción y de las descargas, seguir reproduciendo con la pantalla apagada. |
| Biometría | Bloqueo opcional de la app. |

La app para teléfono no tiene permiso para usar el micrófono.

## Este sitio web

`eveyproject.org` está alojado en GitHub Pages, que ve la dirección IP del
visitante. El sitio sirve sus propias fuentes y scripts, así que el navegador
no contacta con ningún otro servicio. No pone cookies propias ni tiene
estadísticas de visitas.

## Niños

Las apps no están dirigidas a niños. Los perfiles infantiles los crea quien
gestiona el servidor, que es quien guarda esos datos.

## Contacto

Preguntas sobre esta política: [contact@eveyproject.org](mailto:contact@eveyproject.org).
Para los datos guardados en un servidor Evey, ponte en contacto con la
persona que gestiona ese servidor.

<!-- sources
no tracker: frontend/apps/mobile/package.json, frontend/apps/tv/package.json
permissions: frontend/apps/mobile/android/app/src/main/AndroidManifest.xml
device name: frontend/apps/mobile/src/hooks/useDeviceRegistration.ts
playback reports: frontend/packages/client/src/native/useAudioCapsBeacon.ts
outside posters: internal/app/mediarequest/service.go - trustedPosterHosts
cover art: frontend/packages/client/src/lib/artwork.ts - coverArtUri
cast receiver: docs/adr/0025-server-driven-cast.md
voice search: frontend/packages/voice-search/android/src/main/java/com/evey/voicesearch/EveyVoiceSearchModule.kt
store page: frontend/apps/tv/src/lib/appUpdate.ts - openStorePage
-->
