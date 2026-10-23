---
source_sha: d3516f7424f33931631da9e552219eb09b706908443cd9b654577b22f6864c37
reviewed: true
---
# Cómo elige Evey el modo de reproducción { #how-playback-is-decided }

¿Por qué una película se reproduce intacta en la tele pero se convierte en el
teléfono?

**En resumen:**

- Nunca eliges una calidad. Cuando pulsas reproducir, el servidor mira el
  archivo, lo que tu dispositivo puede reproducir y lo ocupado que está.
  Después elige la forma más ligera que funciona.
- Hay tres formas: **Reproducción directa** (el archivo tal cual), **Remux**
  (la misma imagen y el mismo sonido, reempaquetados en otro formato de
  archivo) y **Transcodificación** (una parte de la película convertida, a
  menudo solo el sonido).
- Así, la misma película puede llegar intacta a un dispositivo y convertida a
  otro, o reproducirse de otra forma en el mismo dispositivo cuando el
  servidor está ocupado.
- Normalmente no tienes que hacer nada. Para ver qué forma se ha usado, y
  por qué, abre **Información del flujo** en el reproductor: consulta
  [Leer la Información del flujo](#reading-stream-info).

Primero vienen las tres formas y cómo leer **Información del flujo**.
Después, las reglas exactas que aplica el servidor, para quien quiera
entenderlas o ajustarlas.

## Los tres resultados { #the-three-outcomes }

| En Información del flujo | Lo que recibe el reproductor | Lo que le cuesta al servidor |
| --- | --- | --- |
| **Reproducción directa** | El archivo original, byte a byte. | Leer un archivo. |
| **Remux** | El vídeo y el audio originales en otro contenedor. No se vuelve a codificar nada. | Reempaquetar, cosa que hace [MKVGo](mkvgo.md) dentro del servidor cuando puede, y una copia con FFmpeg en los demás casos. |
| **Transcodificación** | Al menos una pista vuelta a codificar. A menudo solo el audio, con el vídeo todavía original. | Una codificación. Solo volver a codificar el vídeo ocupa una [plaza de transcodificación](transcoding.md#sharing-the-capacity). |

El servidor las prueba en ese orden y, dentro de una transcodificación, solo
vuelve a codificar las pistas que lo necesitan.

## Leer la Información del flujo { #reading-stream-info }

**Información del flujo** (el botón de información, (i), en los controles del
reproductor; en la tele, en el menú de opciones del reproductor) muestra el
**Método**, después cada pista con su decisión (*copiado*,
*transcodificado*, o *descodificado en el cliente* para el audio que
descodifica el propio reproductor web) y, bajo **Por qué no hay reproducción
directa**, los motivos.

<figure class="evey-shot evey-shot--narrow" markdown="span">
  ![Información del flujo en el reproductor web, para un Remux](../images/player-stream-info.webp){ loading=lazy }
  <figcaption>Remux: cambia el empaquetado, las pistas de vídeo y audio quedan intactas, y el panel muestra el único motivo por el que el archivo no se reproduce tal cual. «Copiado» describe las pistas, no el modo de reproducción (interfaz en inglés).</figcaption>
</figure>

Los motivos se reparten en cuatro grupos:

| De dónde viene el motivo | Ejemplos tal como aparecen |
| --- | --- |
| El reproductor no puede usar la pista tal cual | Códec de vídeo que este reproductor no puede descodificar · Códec de audio que este reproductor no puede descodificar · Resolución por encima del límite del reproductor · Más canales de audio de los que admite este reproductor · Formato de píxel que el reproductor no decodifica (H.264 de 10 bits, 4:2:2) · Contenedor no reproducible tal cual (reempaquetado a HLS) |
| Un límite de esta sesión | Tasa de bits por encima del límite permitido · La línea medida por el cliente no puede llevar la copia |
| Una consecuencia para la imagen o el sonido | HDR convertido a SDR (tone-mapping) · Subtítulos incrustados en la imagen · Resincronización de audio solicitada |
| Un ajuste o una decisión ya tomada | Códec de vídeo preferido (ajuste del perfil) · Elección fijada para esta sesión |

Solo cuenta la pista de audio que se está reproduciendo. Un archivo con una
pista de comentarios en DTS que no has elegido no se transcodifica por culpa
de esa pista.

La Información del flujo se calcula cuando empieza la reproducción. El único
caso que no puede prever es una tarjeta gráfica que está llena justo cuando
empieza el flujo: el flujo pasa entonces al procesador, y
**Administración → Reproduciendo ahora** muestra el motor que se usa de
verdad.

## Lo que aporta cada parte { #what-each-side-brings }

**El archivo.** Códec, resolución, profundidad de bits, formato HDR, bitrate
y las pistas de audio y de subtítulos, leídos cuando se escaneó el archivo.

**El reproductor.** Cuando empieza la reproducción, la app indica lo que su
dispositivo puede descodificar: las familias de códecs que admite, los
contenedores que abre, su resolución máxima, si su pantalla muestra HDR,
cuántos canales de audio saca y, en algunos dispositivos, un techo o un
nivel más bajo para H.264. La app web se lo pregunta al navegador; las apps
de Android y Android TV se lo preguntan a los descodificadores y a la
pantalla del dispositivo. Evey no guarda ninguna lista de dispositivos
compatibles: usa lo que dice el reproductor. Una sesión de envío la describe
el propio receptor cuando puede indicar sus capacidades, y un perfil prudente
cuando no: consulta [Enviar a Chromecast](casting.md).

**La sesión.** Las preferencias del perfil (bitrate máximo, canales de audio
máximos, tratamiento de los subtítulos) y los límites que se aplican a este
espectador, más abajo.

**El servidor.** Si puede codificar esto en tiempo real, con su tarjeta
gráfica o con su procesador, y si queda libre una plaza de transcodificación.
Esto se mide, no se supone: consulta
[Transcodificación y capacidad del servidor](transcoding.md).

## Un archivo, tres resultados { #one-file-three-results }

Una película 4K en HEVC y HDR10 a 70 Mb/s, con una pista Dolby TrueHD 7.1,
reproducida en un Android TV cuyo descodificador admite HEVC y cuya pantalla
muestra HDR:

- **Sin límite de bitrate.** La tele descodifica el vídeo y muestra el HDR,
  así que el vídeo se queda tal cual. No descodifica TrueHD, así que solo se
  vuelve a codificar el audio. Información del flujo muestra
  **Transcodificación**, con el vídeo *copiado*.
- **Un límite de 20 Mb/s, y una tarjeta gráfica que codifica HEVC de 10 bits
  con HDR.** Hay que volver a codificar el vídeo para que quepa. El servidor
  lo codifica en HEVC y conserva el HDR, porque la tele y la tarjeta pueden
  hacerlo.
- **El mismo límite, con la tarjeta gráfica ocupada o no disponible para este
  título.** El procesador se encarga. Codifica en H.264, que aquí no
  lleva HDR, así que la imagen se convierte a SDR (tone-mapping), con la
  calidad que el procesador puede mantener.

Ni el archivo ni la tele han cambiado. Han cambiado el límite de la sesión y
el estado del servidor.

## Límites de la sesión { #limits-on-the-session }

Un título cuyo bitrate (el vídeo más la pista de audio que se reproduce)
supera el más bajo de estos límites se transcodifica para bajar hasta él:

- el máximo que indica el reproductor;
- el **Bitrate máximo** de la cuenta, fijado por un administrador;
- el **Bitrate máximo de streaming** del perfil, y su **Bitrate máximo en
  datos móviles** cuando un teléfono indica una conexión móvil;
- el **Bitrate remoto máximo** (**Administración → Acceso remoto**) y el
  **Techo de bitrate de vídeo, espectadores remotos**
  (**Administración → Transcodificación**), el más bajo de los dos, cuando la
  dirección del espectador es pública y está fuera de las **Redes LAN**. Una
  dirección privada nunca se limita.

La pista de audio que se reproduce se vuelve a codificar cuando tiene más
canales de los que indica el reproductor o de los que permiten los **Canales
de audio máximos** del perfil. Una pista vuelta a codificar conserva hasta
5.1 canales, nunca más: consulta [Audio](audio.md).

## Restricciones que aplica el propio servidor { #constraints-the-server-applies-itself }

Hoy los reproductores no indican algunos límites, así que el servidor los
aplica como una regla de compatibilidad prudente, la misma para todos los
reproductores: el H.264 de 10 bits, y cualquier vídeo en 4:2:2, 4:4:4 o de
12 bits, se vuelven a codificar, mientras que HEVC, VP9 y AV1 hasta 10 bits
4:2:0 dependen de los códecs que admita el reproductor. Dolby Vision solo se
conserva en una forma que el reproductor pueda mostrar bien: consulta
[HDR y Dolby Vision](hdr.md).

## Por qué el método casi nunca cambia a mitad de película { #why-the-method-rarely-changes-mid-film }

La decisión se toma cuando empieza la reproducción y se mantiene durante toda
la sesión, para que el reproductor nunca reciba segmentos de dos
codificaciones distintas. Cambiar la pista de audio o de subtítulos de una
forma que lo exija, o volver a empezar el título, provoca una nueva decisión.

## Forzar un método { #forcing-a-method }

No se puede forzar un método: el servidor siempre lo elige según el archivo y el
reproductor. Lo que sí puedes limitar es la calidad, dispositivo por
dispositivo, con **Ajustes → Reproducción → Ahorro de datos (este
dispositivo)**. Limita el flujo solo en ese dispositivo; los demás dispositivos
de la cuenta conservan su propio ajuste. **Desactivado** no pone ningún límite.
**Ahorro** pide como máximo 720p, 3 Mbps y sonido estéreo. **Viaje / pocos
datos** pide como máximo 480p, 1 Mbps y sonido estéreo. Un título por encima del
límite se transcodifica hasta él, e Información del flujo muestra el límite como
motivo.

**Calidad**, en los ajustes del reproductor, es otra cosa: elige entre las
calidades de un flujo transcodificado. **Auto** sigue a la conexión; una
altura (1080p, 720p...) mantiene esa calidad; **Original** quita el límite de
altura que el reproductor pediría si no.

## Para saber más { #going-further }

- Los criterios en detalle: [HDR y Dolby Vision](hdr.md) · [Audio](audio.md) ·
  [Subtítulos](subtitles.md) · [Enviar a Chromecast](casting.md)
- Cómo reparte el servidor su capacidad: [Transcodificación y capacidad del servidor](transcoding.md)
- Lo que funciona dentro del servidor sin FFmpeg: [MKVGo en Evey](mkvgo.md)
- La implementación: [Cadena de reproducción](../playback-serving.md)

<!-- sources
decision: internal/app/transcoding/decision.go - MakePlaybackDecisionWithPrefs, evaluateVideo, pixelFormatPlayable, evaluateAudio, selectedAudioSpec, servedBitrateBps, resolveMaxBitrate, checkBitrate, applyAudioChannelTarget (aacMaxTargetChannels)

session: internal/app/transcoding/playback_plan_resolver.go - Decide, Reported, resolveDecided ; internal/app/transcoding/resolved_route.go - SpillReason (pool_full, title_blocked)
remote cap: internal/adapters/driving/grpcapi/playback_handler.go - resolvePlaybackPrefs ; internal/config/config.go - RemoteBitrateCapKbps
client caps: internal/domain/transcoding/playback_decision.go - ClientProfile ; frontend/packages/client/src/web/codec.ts ; frontend/packages/media-caps/src/castUrl.ts
example: internal/app/transcoding/testdata/route_golden.txt - "nvenc-pass | 2160p-hevc-hdr10-70m | hdr-tv{,-20m} | {plain,gpu-full,title-blocked}"
tone-map reported: internal/app/transcoding/route_tonemap_report_test.go
labels: i18n player.streamInfo.* ; player.quality.* ; auth.settings.playback.* ; admin.yaml maxBitrate (user limit) ; admin.remoteAccess.maxRemoteBitrate, lanNetworks ; admin.calibration.maxBitrateRemoteLabel
entry point: frontend/apps/web/src/features/playback/StreamInfoPanel.tsx ; frontend/apps/web/src/features/playback/PlayerSettingsPanel.tsx ; frontend/apps/mobile/src/components/player/PlayerControlsRow.tsx ; frontend/apps/tv/src/components/TrackOptionsMenu.tsx
adr: 0024, 0029, 0030
plain opening: docs/playback/transcoding.md ; i18n player.streamInfo.title
-->
