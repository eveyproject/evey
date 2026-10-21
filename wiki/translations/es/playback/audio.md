---
source_sha: c8146d533df94a1909ae91a9bbceb788ab4d86f1860b721414c4dadf8791cff9
reviewed: true
---
# Audio

**En resumen:**

- Oyes el idioma de tu perfil cuando la película lo tiene: elígelo en
  **Ajustes → Reproducción → Idioma de audio por defecto**. Una pista que
  eliges a mano en el reproductor siempre tiene prioridad.
- El sonido se envía tal cual cuando tu dispositivo puede reproducirlo. Si
  no, el servidor lo convierte a un formato común (AAC). A menudo el sonido
  es lo único que se convierte, y la imagen queda intacta.
- En el altavoz de un teléfono, con auriculares o por Bluetooth, la app de
  Android reproduce en estéreo. Con la app de Android o de Android TV por
  HDMI, recibes tantos canales como acepte la tele o el amplificador (5.1,
  por ejemplo).
- ¿El sonido no va sincronizado con la imagen? Consulta
  [Sonido desincronizado](#sound-out-of-sync).

El resto de esta página explica qué pista se reproduce, cuándo se copia o se
convierte, y con cuántos canales. Los ajustes del perfil que se mencionan
aquí están en **Ajustes → Reproducción**.

## Lo que oirás, según el dispositivo

| Tu dispositivo | Lo que llega |
| --- | --- |
| Un navegador | La pista tal cual cuando el navegador la descodifica; AC-3 y E-AC-3 descodificados en la propia página (más abajo); si no, AAC. Una pista AAC envolvente la reduce el navegador a tu salida de audio. |
| Un teléfono Android con su altavoz, auriculares o Bluetooth | Estéreo. La app declara una salida estéreo, así que el servidor vuelve a codificar una pista envolvente en estéreo, con una mezcla de volumen equilibrado. El Dolby Digital que descodifica la propia app se envía tal cual y lo mezcla la app. |
| Un teléfono Android o un Android TV por HDMI | Tantos canales como indica que acepta el equipo conectado por HDMI: una pista 5.1 se queda en 5.1 con un amplificador 5.1. |
| Un receptor de envío | Lo que el receptor ha indicado sobre su salida: consulta [Enviar a Chromecast](casting.md). |

Las apps de Android comprueban la salida de audio cada vez que empieza una
reproducción: si conectas unos auriculares, el cambio se nota en el siguiente
título, cambio de pista o cambio de calidad, no en mitad de una escena.

## Qué pista se reproduce

Una pista elegida a mano, en la página del título o en el reproductor,
siempre tiene prioridad. Si no, los reproductores siguen el **Idioma de audio
por defecto** del perfil, después la pista que el archivo marca como
predeterminada, y después la primera. Una pista marcada como comentario se
salta en favor de una normal cuando el archivo tiene las dos.

Solo cuenta la pista que se está reproduciendo: una pista DTS que no estás
escuchando nunca provoca una transcodificación.

## Copiada o vuelta a codificar

La pista se copia cuando el reproductor descodifica su códec y su salida
tiene canales suficientes, dentro de los **Canales de audio máximos** del
perfil (**Estéreo (2.0)**, **Surround (5.1)** o **Surround (7.1)**). Si no, se
vuelve a codificar en AAC:

- con tantos canales como permitan la fuente, el dispositivo y el perfil,
  hasta 5.1 (una fuente 7.1 se reduce a 5.1, que descodifican todos los
  reproductores; las fuentes estéreo siguen en estéreo);
- sea cual sea la frecuencia de muestreo, que nunca es por sí sola un motivo
  para volver a codificar.

Una pista copiada dentro de un flujo transcodificado o en remux también tiene
que estar en un formato que el reproductor acepte dentro de los segmentos del
flujo. AAC, AC-3 y E-AC-3 siempre lo están; los demás códecs, solo cuando el
reproductor lo indica. Cuando no es así, la pista se vuelve a codificar e
Información del flujo dice **La pista de audio no puede transmitirse sin
cambios en el stream**: un reproductor puede descodificar MP3 en un archivo y
aun así rechazarlo dentro de un flujo.

## Dolby Digital en un navegador

La mayoría de los navegadores no pueden descodificar AC-3 ni E-AC-3. En lugar
de volver a codificar la pista en el servidor, el reproductor web la
descodifica él mismo, con un descodificador que funciona dentro de la página:
la pista se envía sin cambios e Información del flujo la muestra como
*descodificado en el cliente*. Los navegadores que descodifican Dolby Digital
por sí mismos, como Safari, la reciben como una pista normal; la elección se
hace probando el navegador, no reconociéndolo. Una pista envolvente solo se
descodifica en la página cuando los **Canales de audio máximos** del perfil
lo permiten: un perfil en estéreo recibe en su lugar la versión en AAC, que
puede reducir los canales.

Cómo la descodifica la página y qué pasa cuando no puede:
[ac3go en Evey](ac3go.md).

!!! warning
    Un proxy inverso que pone su propia Content-Security-Policy debe
    permitir `'wasm-unsafe-eval'` en `script-src`; si no, el descodificador
    no puede arrancar y la película se reproduce sin sonido. Consulta
    [Configuraciones de proxy inverso](../reverse-proxy.md#security-headers--csp---read-this-if-your-proxy-sets-its-own).

## Sonido desincronizado

**Sincronización de audio**, en el reproductor, adelanta o retrasa el sonido
del título actual (los valores positivos lo retrasan). Se ofrece cuando el
flujo llega del servidor en remux o transcodificado; en Reproducción directa,
el control indica **Disponible solo en reproducción transcodificada**. Aplicar
un desfase vuelve a arrancar el flujo en la posición actual. Una pista
copiada no se puede desplazar, así que el servidor la vuelve a codificar e
Información del flujo muestra **Resincronización de audio solicitada**.

Cuando el desfase está en el propio archivo (una pista de audio que empieza
después de la imagen, o pistas que no coinciden entre sí),
**Administración → Salud de medios** lo señala pero nunca lo reescribe por su
cuenta: un desfase al principio suele ser un ajuste deliberado del doblaje.
**Comprobar sincronía**, en la fila del archivo, te deja juzgarlo a ojo y
aplicar un desplazamiento, que su historial puede deshacer. Consulta
[Salud de medios](../runbooks/media-health.md#checking-the-sync-by-eye).

<!-- sources
track choice: internal/app/transcoding/decision.go - selectedAudioSpec ; frontend/packages/client/src/hooks/usePlayerTrackSelection.ts
copy vs aac, channels: internal/app/transcoding/decision.go - evaluateAudio, applyAudioChannelTarget (aacMaxTargetChannels)
device channel ceiling: frontend/packages/client/src/native/outputChannels.ts - resolveMaxOutputChannels (STEREO_ONLY_TYPES, hdmi.maxChannelCount) ; frontend/packages/client/src/native/playSession.ts - playerDecodesCopiedAudio ; internal/adapters/driving/http/client_capabilities.go - maxAudioChannelsQuery
passthrough in segments: internal/app/transcoding/decision.go - applyPackagedAudioPassthrough ; internal/app/transcoding/source_facts.go - AudioCopyCodecAllowedFor ; internal/domain/transcoding/playback_decision.go - ClientProfile.FMP4AudioPassthrough
wasm: internal/app/transcoding/wasm_audio.go - WasmAudioCodecSupported, WasmAudioChannelsOK ; internal/app/transcoding/decision.go - wasmAudioApplies ; frontend/packages/client/src/web/codec.ts
sample rate: internal/app/transcoding/decision.go - evaluateAudio (no sample-rate check)
resync: internal/app/transcoding/playback_plan_resolver.go - Decide (audio_resync_requested) ; frontend/apps/web/src/features/playback/HLSPlayer.tsx (audioOffsetAvailable=transcodeMode) ; frontend/apps/mobile/app/(auth)/player/[id].tsx (audioOffsetAvailable)
media health: i18n admin.syncPreview.btn ; docs/runbooks/media-health.md
labels: i18n auth.settings.playback.maxAudioChannels, channelsStereo, channels51, channels71, audioLanguage ; player.settings.audioSync, audioSyncHint, audioSyncUnavailable ; player.streamInfo.decisionWasm, reason.audio_passthrough_unsupported, reason.audio_resync_requested
plain opening: frontend/packages/client/src/hooks/usePlayerTrackSelection.ts ; i18n auth.settings.playback.audioLanguage
-->
