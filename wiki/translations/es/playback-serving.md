---
source_sha: 8e9b10f4cdc162d51247a1432d8c6b2a3ab2490d0c01a856d52961436aa38556
---

# Servir un vídeo: la vía de reproducción HLS

Esta es la **vía de lectura**: qué ocurre cuando un reproductor pide un vídeo,
desde la primera petición de `master.m3u8` hasta los bytes de un segmento. Es la
contraparte de la cadena de análisis descrita en
[ARCHITECTURE.md](ARCHITECTURE.md) (la vía de escritura, la que prepara el modelo
de lectura) y el detalle que hay detrás del «Golden path 4» de
[CODEBASE_MAP.md](CODEBASE_MAP.md).

La vía de reproducción es **VITAL** y está repartida entre unos 8 archivos del
adaptador `http`. No existe ninguna función que ejecute el flujo entero: el mapa
es este documento.

## El modelo mental: 3 preguntas, 2 motores, 1 clave

Servir un vídeo se reduce a responder tres preguntas y después empaquetar la
respuesta, con todo direccionado por un mismo hash:

- **P1 - ¿Puedo leer el archivo sin que cueste?** El reparto del sondeo, según la
  **extensión del archivo**: `.mkv/.webm` y `.mp4/.mov` van a **mkvgo** (solo la
  cabecera, sin lanzar ningún proceso); todo lo demás, y cualquier fallo de la
  vía rápida, va a **ffprobe** (la autoridad).
- **P2 - ¿Puede el cliente reproducirlo tal cual?** La **decisión** de
  reproducción, tomada *pista a pista*: para el vídeo, `copy` o `transcode`; para
  el audio, `copy`, `transcode` o la vía wasm de AC-3. Vídeo y audio se deciden
  **por separado**, y el caso más habitual es «copiar el vídeo y recodificar el
  audio a AAC».
- **P3 - ¿Cuánto puede codificar esta máquina?** La **capacidad** calibrada (que
  sale del benchmark del arranque) más la **admisión** (qué peldaño de la
  escalera cabe y si queda plaza).

Por todo ello atraviesan dos motores:

- **mkvgo** - lee y reempaqueta (remux o bajo demanda) sin lanzar ningún proceso. **Gratis.**
- **ffmpeg / ffprobe** - descodifica, recodifica y sondea lanzando un proceso. **Caro.**

Y una clave: todo (el directorio de caché, la sesión de ffmpeg, las URL de los
segmentos) se direcciona mediante la **huella de la decisión**, que es un hash de
esa decisión. Los mismos bytes ⇒ el mismo hash ⇒ una única codificación
compartida. La corrección de todo el sistema descansa en que ese hash sea
**estable** entre el manifiesto que tiene el reproductor y cada segmento que
pedirá después.

## Orden de lectura: siga una petición a través del código

Abra estos archivos en este orden para seguir una sola petición de reproducción
de principio a fin:

1. **Entrada** · `internal/adapters/driving/http/transcode_handler_vod.go` -
   `serveHTTP` autentica y reparte después hacia el maestro, la variante, el
   segmento o el audio.
2. **P2 · Decisión** · `internal/app/transcoding/decision.go` -
   `MakePlaybackDecisionWithPrefs` (función pura): el cerebro que decide copiar o
   transcodificar cada pista. Lo envuelve `makeDecision`, en
   `http/transcode_vod_fingerprint.go`, que le añade los anclajes del momento de
   la petición (`vd=` y `ad=`), la anulación de resincronización de audio, etc.
3. **P3 · Capacidad** · `internal/app/transcoding/calibration_service.go` -
   `Current()` devuelve `domain/transcoding.Derive(report, policy)`: los techos
   de la máquina, derivados del último benchmark.
4. **Modelado de la escalera** · `http/transcode_vod_profiles.go` -
   `profilesForDecision`: clase de complejidad → perfiles de productor → topes de
   la fuente y de la capacidad → topes de CMCD y AV1 → ruta y techo de admisión,
   y después **congela la escalera ya modelada** en el anclaje de admisión (que
   se lee tal cual en cada llamada posterior: el invariante del hash de caché
   convertido en estructura).
5. **Admisión** · `internal/app/transcoding/admission_shaper.go`
   (el `StreamPlan` inmutable) más `video_session_limiter.go`: la ruta (con el
   desbordamiento de hardware a software) y la plaza de capacidad.
6. **Huella** · `http/transcode_vod_fingerprint.go` (`buildFingerprint`) compone
   una `DecisionFingerprint` (el tipo y su `Hash()` están en
   `app/transcoding/segment_cache.go`). Ese hash es la clave de caché.
7. **Producir los bytes** · copia → **mkvgo**, `app/transcoding/ondemand.go` (el
   puerto) y `adapters/driven/mkvgo/ondemand.go`; transcodificación → **ffmpeg**,
   `adapters/driven/ffmpeg/hls_transcoder.go` (`BuildFFmpegArgs` es pura; los
   segmentos aterrizan en `/cache/transcode/segments/<hash>/`).
8. **Empaquetado** · `app/transcoding/manifest_generator.go` - `master.m3u8` y
   `stream_N.m3u8` (más `manifest.mpd` en la vía de mkvgo).

## Los dos motores, por lo que cuestan

El **repartidor de sondeos** (`adapters/driven/prober/dispatcher.go`) es una
cadena de responsabilidad que se guía por la extensión: intenta mkvgo y recae en
ffprobe ante la menor duda (un error de análisis, un pánico, un resultado sin
correspondencia, una pista multimedia MP4 descartada, un perfil 5 de Dolby
Vision, datos de color ilegibles). Las dos vías rápidas están siempre activas (no
hay ninguna puerta que las apague: son `nil` en `wire.go`); ffprobe es la red de
seguridad y nunca se salta. Un segundo repartidor idéntico
(`keyframe_dispatcher.go`) se ocupa del índice de posicionamiento.

| Operación | mkvgo (gratis) | ffprobe / ffmpeg (caro) |
|---|---|---|
| Sondeo de metadatos (códec, dimensiones, HDR, tasa de bits, duración) | `.mkv/.webm/.mp4/.mov`, solo la cabecera | todo lo demás, o el repliegue |
| Modo de tasa de fotogramas (cfr/vfr) | `AnalyzeFrameRateMode` | - (ya migrado) |
| Remux de MKV a MP4 (sin recodificar) | `RemuxToMP4` | - |
| HLS/DASH bajo demanda (peldaño de copia) | `OnDemandPackager` | - |
| Recodificación de vídeo y mapeo tonal de HDR | - | `hls_transcoder.go` |
| Detección de entrelazado (idet), punto de aterrizaje de un posicionamiento, chromaprint, perfil 5 de Dolby Vision | - (hace falta descodificar o leer datos laterales) | ffmpeg / ffprobe |

Todavía se lanzan tres procesos ffprobe sobre fuentes MKV y MP4 cuyos datos mkvgo
ya expone (la duración en `trickplay.go`, la tasa de bits y las dimensiones en
`complexity_classifier.go`, el retardo de audio de la reproducción en
`audio_delay.go`): son candidatos a migrar, con poca ganancia de CPU y sobre todo
cableado.

## El audio se decide aparte del vídeo

`makeDecision` produce una `VideoDecision` y una `AudioDecision` de forma
independiente. El «peldaño de copia» (`isCopyRequest`) solo depende del **vídeo**;
incluso un vídeo copiado suele llevar al lado una pista AAC recodificada. La
decisión de audio tiene tres desenlaces: copiar, transcodificar a AAC (con
ffmpeg, poca cosa) o, para **AC-3 y E-AC-3** en un navegador que no sabe
descodificarlos de forma nativa, la **vía wasm**: el maestro se deja solo con
vídeo y una etiqueta `#EVEY-AUDIO-URI`, el AC-3 se **copia** (con el `od_audio`
de mkvgo si el plan se puede construir, y si no con un `-c:a copy` de ffmpeg) y
`ac3go.wasm` lo descodifica a PCM en el navegador (`WasmAudioEngine.ts`),
sincronizado con el reloj del vídeo. Safari queda fuera por capacidad, no por su
nombre: `canWasmDecodeAC3()` solo devuelve verdadero cuando el navegador declara
que *no* puede reproducir AC-3 de forma nativa, y Safari declara que sí puede.

## Empaquetado: CMAF, HLS, DASH

El contenedor es siempre **CMAF** (fMP4): un `init.mp4` por variante más los
segmentos `.m4s`. (La rama MPEG-TS de `hls_transcoder.go` está muerta:
`needsFMP4Segments` devuelve siempre verdadero.) Sobre *esos mismos* segmentos:

- **HLS** (`master.m3u8` → `stream_N.m3u8` → el `EXT-X-MAP` de inicialización más
  los `.m4s`) - en los dos motores.
- **DASH** (`manifest.mpd`) - **solo en la vía de copia de mkvgo**
  (`OnDemandPlan.HasDASH`), idéntico byte a byte y sin coste de almacenamiento;
  no está en la vía de transcodificación de ffmpeg y se desactiva cuando el
  cifrado AES-128 está activo.

Las URL de cada sesión se vuelven a grabar
(`transcode_vod_manifest_rebake.go`) para que lleven el token del flujo, tanto en
HLS como en DASH, incluido el Chromecast, que no lleva cookies.

## Patrones de diseño en juego

- **Puertos y adaptadores (arquitectura hexagonal)** - la capa de aplicación
  habla con interfaces (`StreamProber`, `BenchmarkRunner`, `OnDemandPackager`,
  `StreamProducer`) que implementan ffmpeg y mkvgo. Un motor nuevo es un
  adaptador nuevo.
- **Estrategia** - `StreamProducer` (software, hardware o copia), que elige un
  `ProducerSelector`.
- **Cadena de responsabilidad con repliegue** - los repartidores de sondeos y de
  fotogramas clave (mkvgo → la autoridad de ffprobe).
- **Caché direccionada por contenido** - el hash de la `DecisionFingerprint` es la
  clave tanto de la caché como de la sesión de ffmpeg.
- **Núcleo puro y cáscara imperativa** - `Derive`, `MakePlaybackDecision` y el
  modelo de coste son funciones puras, con pruebas unitarias que no necesitan ni
  ffmpeg ni base de datos.
- **Objeto de valor inmutable** - `StreamPlan`: la decisión de admisión, emitida
  una sola vez y nunca modificada, de modo que el invariante entre las URL y los
  bytes se cumple por construcción.
- **Degradación elegante** - hay un repliegue seguro en cada paso: capacidad de
  720p por defecto, desbordamiento de hardware a software, de copia a
  transcodificación, de mkvgo a ffprobe.

## El cerebro de la decisión: el PlanResolver de la capa de aplicación

El cerebro que atiende la petición (los anclajes de la decisión, el modelado de
la escalera, el encaminamiento por hardware, la composición de la huella, el
encaminamiento del peldaño de copia, la elección del productor) vive en la capa
de aplicación: `PlanResolver` (`app/transcoding/playback_plan_resolver.go`) y su
implementación concreta `PlaybackPlanResolver`
([`playback_plan.go`](../internal/app/transcoding/playback_plan.go)). Cada
petición se resuelve **una sola vez**: el adaptador http traduce el
`*http.Request` a una `PlaybackRequest` sin dependencia del transporte
(`http/transcode_vod_plan.go`) y sirve a partir del `PlaybackPlan` inmutable que
recibe. Todos los consumidores (la entrada del manifiesto, el sondeo de descarga,
el sellado de las URL caducadas, la elección del productor, la clave del
segmento) leen esa misma resolución, así que *dentro de una petición* el
invariante del hash de caché se cumple por construcción (y la vía caliente de los
segmentos pasó de unas 5 consultas de preferencias de perfil a 1). Las funciones
auxiliares que exponen los hechos de la fuente (códec, HDR, SAR, diez bits…) están
al lado, en `app/transcoding/source_facts.go`; el adaptador se limita a mantener
unos alias finos. De una petición a otra, el invariante sigue descansando en el
grabado de las URL: **toda entrada de la decisión tiene que quedar grabada en las
URL hijas** o un sondeo a mitad de reproducción puede cambiarle el plan al
reproductor por debajo (`querySuffix`, en `manifest_generator.go`).
