---
source_sha: fbe5c63d26023c75b078bb43d1151792e32f425b107270e73ddeebd67c742e4a
reviewed: false
---
# Servir un vídeo: la ruta de reproducción HLS

Esta es la **ruta de lectura**: lo que pasa cuando un reproductor pide un vídeo,
desde la primera petición de `master.m3u8` hasta los bytes de un segmento. Es la
contraparte de la canalización de escaneo de [ARCHITECTURE.md](ARCHITECTURE.md)
(la ruta de escritura que prepara el modelo de lectura), y el detalle que hay
detrás del "Golden path 4" de [CODEBASE_MAP.md](CODEBASE_MAP.md).

La ruta de reproducción es **VITAL** y está repartida en ~8 archivos del
adaptador `http`. No hay una sola función que ejecute todo el flujo: este
documento es el mapa.

## El modelo mental: 3 preguntas, 2 motores, 1 clave

Servir un vídeo se reduce a responder tres preguntas y empaquetar la respuesta,
con todo direccionado por un único hash:

- **P1 - ¿Puedo leer el archivo de forma barata?** Reparto del sondeo según la
  **extensión del archivo**: `.mkv/.webm` y `.mp4/.mov` van a **mkvgo** (solo la
  cabecera, sin proceso); todo lo demás, y cualquier fallo de la vía rápida, va a
  **ffprobe** (la autoridad).
- **P2 - ¿Puede el cliente reproducirlo tal cual?** La **decisión** de
  reproducción, tomada *por pista*: vídeo `copy` o `transcode`, audio `copy`,
  `transcode` o la vía AC-3 wasm. El vídeo y el audio se deciden de forma
  **independiente**: el caso habitual es "copiar el vídeo, recodificar el audio a
  AAC".
- **P3 - ¿Cuánto puede codificar este equipo?** La **capacidad** calibrada (de la
  medición hecha en el primer momento tranquilo) más la **admisión** (qué escalón
  de la escala cabe, si hay plaza).

Dos motores atraviesan todo:

- **mkvgo**: lee y reempaqueta (remux / bajo demanda) sin lanzar ningún proceso. **Gratis.**
- **ffmpeg / ffprobe**: descodifica, recodifica y sondea lanzando un proceso. **Caro.**

Una clave: todo (directorio de caché, sesión de ffmpeg, URL de los segmentos) se
direcciona por la **huella de la decisión**, un hash de la decisión. Mismos bytes
⇒ mismo hash ⇒ una codificación compartida. La corrección de todo el sistema
descansa en que ese hash sea **estable** entre el manifiesto que tiene un
reproductor y cada segmento que pide después.

## Orden de lectura: seguir una petición por el código

Abre estos archivos en orden para seguir una petición de reproducción de
principio a fin:

1. **Entrada** · `internal/adapters/driving/http/transcode_handler_vod.go`:
   `serveHTTP` autentica y luego reparte entre master / variante / segmento / audio.
2. **El plan** · `http/transcode_vod_plan.go` convierte la petición en un
   `PlaybackRequest`; `app/transcoding/playback_plan_resolver.go`:
   `PlanResolver.Resolve` devuelve un único `PlaybackPlan` inmutable: la decisión
   (P2), la clase de escala, la escala ajustada, la ruta de hardware, el
   empaquetador y la huella. Cada manifiesto y cada segmento de la sesión se
   sirven a partir de él ([ADR-0030](adr/0030-one-plan-one-decision.md)).
   `makeDecision` y `buildFingerprint` en `http/transcode_vod_fingerprint.go` son
   envoltorios finos para los puntos de llamada sin plan (precalentamiento,
   master bajo demanda).
3. **P2 · Decisión** · `internal/app/transcoding/decision.go`:
   `MakePlaybackDecisionWithPrefs` (pura): el cerebro de copia/transcodificación
   por pista. El resolvedor añade encima los anclajes de la petición
   (`vd=` / `ad=`), la anulación de resincronización de audio, etc.
4. **P3 · Capacidad** · `internal/app/transcoding/calibration_service.go`:
   `Current()` devuelve `domain/transcoding.Derive(report, policy)`
   (`derive.go`): los techos del equipo, deducidos de la última calibración.
5. **Escala y admisión** · el resolvedor ajusta la escala (clase de complejidad
   → perfiles de productor → límites de fuente y capacidad → límites CMCD/AV1 →
   ruta y techo de admisión) y después la **congela** en el anclaje de admisión
   (`admission_shaper.go`, `FreezeLadder`), que se lee tal cual en cada llamada
   posterior: la invariante del hash de caché convertida en estructura.
   `video_session_limiter.go` guarda la plaza de capacidad; la ruta pasa del
   hardware al software cuando está lleno.
6. **Huella** · un `DecisionFingerprint` (tipo + `Hash()` en
   `app/transcoding/segment_cache.go`), ensamblado por el resolvedor. El hash es
   la clave de la caché.
7. **Producir los bytes** · copia → **mkvgo** `app/transcoding/ondemand.go` (puerto) /
   `adapters/driven/mkvgo/ondemand.go`; transcodificación → **ffmpeg**
   `adapters/driven/ffmpeg/hls_transcoder.go` (`BuildFFmpegArgs` es pura; los
   segmentos van a `/cache/transcode/segments/<hash>/`).
8. **Empaquetado** · `app/transcoding/manifest_generator.go`: `master.m3u8` /
   `stream_N.m3u8`; `manifest_generator_dash.go`: `manifest.mpd`.

## Los dos motores, por coste

El **repartidor de sondeos** (`adapters/driven/prober/dispatcher.go`) es una
cadena de responsabilidad según la extensión: prueba mkvgo y recurre a ffprobe
ante cualquier duda (error de análisis, panic, resultado no traducible, una pista
multimedia MP4 descartada, DoVi perfil 5, color ilegible). Las dos vías rápidas
están siempre activas (puertas nil en `wire.go`); ffprobe es la red de seguridad,
nunca se salta. Un segundo repartidor idéntico (`keyframe_dispatcher.go`) se
ocupa del índice de posicionamiento.

| Operación | mkvgo (gratis) | ffprobe / ffmpeg (caro) |
|---|---|---|
| Sondeo de metadatos (códec, dimensiones, HDR, bitrate, duración) | `.mkv/.webm/.mp4/.mov` solo la cabecera | todo lo demás, o respaldo |
| Modo de frecuencia de imagen (cfr/vfr) | `AnalyzeFrameRateMode` | - (migrado) |
| Remux MKV→MP4 (sin recodificar) | `RemuxToMP4` | - |
| HLS/DASH bajo demanda (escalón de copia) | `OnDemandPackager` | - |
| Recodificación de vídeo / mapeo de tonos HDR | - | `hls_transcoder.go` |
| Detección de entrelazado (idet), aterrizaje de saltos, chromaprint, DoVi P5 | - (necesita descodificar / datos laterales) | ffmpeg / ffprobe |

Tres llamadas a ffprobe siguen ejecutándose sobre fuentes MKV/MP4 cuyos datos ya
expone mkvgo (duración en `trickplay.go`, bitrate+dimensiones en
`complexity_classifier.go`, retardo de audio de reproducción en `audio_delay.go`):
candidatas a migrar; poca ganancia de CPU, sobre todo conexiones.

## El audio se decide por separado del vídeo

`makeDecision` produce `VideoDecision` y `AudioDecision` de forma independiente.
El "escalón de copia" (`isCopyRequest`) depende solo del **vídeo**; incluso un
vídeo copiado suele llevar al lado una pista AAC recodificada. La decisión de
audio tiene tres resultados: copia, transcodificación a AAC (ffmpeg, ligera) o,
para **AC-3 / E-AC-3** en un navegador que no puede descodificarlo de forma
nativa, la **vía wasm**: el master se hace solo de vídeo con una etiqueta
`#EVEY-AUDIO-URI`, el AC-3 se **copia** (mkvgo `od_audio` si el plan se
construye, si no ffmpeg `-c:a copy`), y `ac3go.wasm` lo descodifica a PCM en el
navegador (`WasmAudioEngine.ts`), sincronizado con el reloj del vídeo. Safari
queda excluido por capacidad, no por nombre: `canWasmDecodeAC3()` solo devuelve
true cuando el navegador indica que *no puede* reproducir AC-3 de forma nativa, y
Safari indica que sí puede.

## Empaquetado: CMAF, HLS, DASH

El contenedor es siempre **CMAF** (fMP4): un `init.mp4` por variante más
segmentos `.m4s`. (La rama MPEG-TS de `hls_transcoder.go` está muerta:
`needsFMP4Segments` siempre devuelve true.) Sobre los *mismos* segmentos:

- **HLS** (`master.m3u8` → `stream_N.m3u8` → init `EXT-X-MAP` + `.m4s`): ambos motores.
- **DASH** (`manifest.mpd`): en la vía de copia de mkvgo, el propio manifiesto del
  empaquetador (`OnDemandPlan.HasDASH`, idéntico byte a byte, sin almacenamiento,
  desactivado con AES-128); en la vía de ffmpeg, un manifiesto **declarado a partir
  del plan** sobre los mismos fragmentos a los que apuntan las listas HLS
  (`ManifestGenerator.MPD`), escrito antes de que exista ningún fragmento. Lo que
  lo hace honesto es la **rejilla de segmentos** (`domain/transcoding.SegmentGrid`):
  cada escalón transcodificado se corta en el mismo número entero de fotogramas
  (72 a 23,976 fps, 75 a 25, 90 a 29,97), así que un segmento es exacto en los
  ticks de la pista de vídeo e idéntico en todo el conjunto de conmutación; el
  respaldo de copia declara su plan de fotogramas clave. Una fuente cuya
  frecuencia de imagen es desconocida o variable se queda en el antiguo reloj de
  3 s, conserva HLS y responde 404 a `manifest.mpd`. Ningún cliente pasa a DASH:
  HLS sigue siendo el predeterminado en todas partes. El `DASHFromCMAF` de mkvgo
  sobre los fragmentos producidos es el oráculo de paridad
  (`internal/pkg/cmafparity`, `TestDASHGridIntegration` a 23,976/25/29,97 fps: en
  frío, salto, conjunto de conmutación de dos escalones, audio aparte dentro de un
  fotograma AAC).

Las URL de cada sesión se reescriben (`transcode_vod_manifest_rebake.go`) para
llevar el token del flujo, tanto en HLS como en DASH, incluido el Chromecast sin
cookies.

## Patrones de diseño en juego

- **Puertos y adaptadores (hexagonal)**: la capa de aplicación habla con
  interfaces (`StreamProber`, `BenchmarkRunner`, `OnDemandPackager`,
  `StreamProducer`); ffmpeg y mkvgo las implementan. Un motor nuevo es un
  adaptador nuevo.
- **Estrategia**: `StreamProducer` (software / hardware / copia), elegido por un
  `ProducerSelector`.
- **Cadena de responsabilidad con respaldo**: los repartidores de sondeos y de
  fotogramas clave (mkvgo → autoridad ffprobe).
- **Caché direccionada por contenido**: el hash del `DecisionFingerprint` es la
  clave de la caché y de la sesión de ffmpeg.
- **Núcleo puro / envoltorio imperativo**: `Derive`, `MakePlaybackDecision` y el
  modelo de coste son puros y se prueban sin ffmpeg ni base de datos.
- **Objeto de valor inmutable**: `StreamPlan`, la decisión de admisión, emitida una
  vez y nunca modificada, para que la invariante URL↔bytes se cumpla por
  construcción.
- **Degradación elegante**: un respaldo seguro en cada paso: capacidad por defecto
  de 720p, paso de hardware a software, copia→transcodificación, mkvgo→ffprobe.

## El cerebro de la decisión: el PlanResolver de la capa de aplicación

El cerebro que trata las peticiones (anclajes de decisión, ajuste de la escala,
ruta de hardware, ensamblado de la huella, encaminamiento del escalón de copia,
elección del productor) vive en la capa de aplicación: `PlanResolver`
(`app/transcoding/playback_plan_resolver.go`), el `PlaybackPlanResolver` concreto
([`playback_plan.go`](../internal/app/transcoding/playback_plan.go)). Cada
petición se resuelve **una vez**: el adaptador http convierte el `*http.Request`
en un `PlaybackRequest` sin transporte (`http/transcode_vod_plan.go`) y sirve a
partir del `PlaybackPlan` inmutable devuelto. Todos los consumidores (entrada del
manifiesto, sonda de descarte, sellado de URL obsoletas, elección del productor,
clave del segmento) leen esa única resolución, así que *dentro de una petición*
la invariante del hash de caché se cumple por construcción (y la ruta caliente de
los segmentos pasó de ~5 lecturas de preferencias del perfil a 1). Los auxiliares
de hechos de la fuente (códec, HDR, SAR, diez bits…) viven al lado en
`app/transcoding/source_facts.go`; el adaptador conserva alias finos. Entre
peticiones la invariante sigue descansando en lo que se incrusta en las URL: **cada
entrada de la decisión debe quedar incrustada en las URL hijas**, o un sondeo a
mitad de reproducción puede cambiar el plan bajo el reproductor (`querySuffix` en
`manifest_generator.go`).
