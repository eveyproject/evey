---
source_sha: e039606b33ab2207d3774f9bacb71ee3d18f0d12f52d9dc02bec1e9817e1c91e
reviewed: true
---
# Transcodificación y capacidad del servidor { #transcoding-and-server-capacity }

**En resumen:**

- **Transcodificar** es convertir un vídeo a una forma que el dispositivo
  pueda reproducir. Es el único trabajo pesado que hace el servidor:
  reproducir un archivo tal cual (Reproducción directa) o reempaquetarlo
  (Remux) no cuesta casi nada. Consulta
  [Cómo elige Evey el modo de reproducción](how-playback-is-decided.md).
- No eres tú quien fija a cuántas personas puede atender el servidor. Se mide
  a sí mismo en su primer momento tranquilo, en unos minutos, y nunca acepta
  más.
- Cuando le falta capacidad, una reproducción nueva recibe una calidad más
  baja, o pasa de una tarjeta gráfica ocupada al procesador. Si ninguno de
  los dos tiene sitio, el reproductor dice **El servidor está ocupado
  transcodificando otros flujos**.
- Lo que quizá quieras hacer: darle al servidor una tarjeta gráfica
  ([Aceleración por hardware](../administration/hardware-acceleration.md)) y
  dejar espacio libre en el disco que guarda la caché
  ([La caché de transcodificación](#the-transcoding-cache)).

El resto de esta página es para el administrador que quiere los detalles:
cómo mide el servidor lo que puede codificar, cómo reparte esa capacidad y
qué ve un espectador cuando está lleno.

## El servidor se mide a sí mismo { #the-server-measures-itself }

La capacidad no es un ajuste que tengas que estimar tú. El servidor la
mide:

- **Cuándo.** No al arrancar: el servidor arranca con lo que ya sabía (o, la
  primera vez, con un valor prudente de 720p por software) y hace la medición
  en su primer momento tranquilo, cuando nadie está viendo nada y el
  procesador está casi parado. Tarda unos minutos. Una medición hecha con la
  máquina cargada se marca como provisional y se repite cuando la máquina
  está libre.
- **Qué.** Codificaciones reales de un clip 4K, en SDR y en HDR, con el
  procesador y con cada acelerador que el servidor ha detectado, de la misma
  forma que una reproducción real. El resultado es la resolución más alta que
  cada uno soporta en tiempo real, y lo que cuesta cada tipo de flujo.
- **Otra vez.** Cuando cambia el hardware, o con **Recalibrar** en
  **Administración → Transcodificación → Calibración de codificación**. El
  botón se niega mientras alguien está viendo algo; solo la nueva medición automática espera a que el servidor esté libre.

Mientras dura la medición, una reproducción que necesita transcodificar
muestra **El servidor se está calibrando** y se le pide que lo vuelva a
intentar en un momento. La Reproducción directa y el Remux no se ven
afectados.

En un procesador que no soporta ninguna de las resoluciones, el servidor
sigue ofreciendo 720p (SDR) y 480p (HDR), cuántos flujos permite a la vez sale del presupuesto del limitador de sesiones de vídeo.

La medición usa, por este orden, un clip que le des tú, un clip de referencia
que se descarga una vez y se comprueba con su SHA256, o un clip sintético
generado en el momento. Un clip real mide de forma más fiel que el
sintético, que exagera el coste y puede poner a una máquina un límite más
bajo del que le corresponde:

```yaml title="docker-compose.yml"
services:
  evey:
    environment:
      EVEY_BENCH_HDR_SAMPLE: /data/bench/my-4k-hdr.hevc
      EVEY_BENCH_SDR_SAMPLE: /data/bench/my-4k-sdr.hevc
```

## Repartir la capacidad { #sharing-the-capacity }

**Capacidad de reproducción en directo** (**Administración →
Transcodificación**) es el número de flujos de vídeo que se pueden codificar
a la vez. Se deduce de la medición y se ajusta sola cuando cambia el
hardware. **Limitar los flujos de vídeo simultáneos** puede bajarla,
nunca subirla.

- Una transcodificación que solo vuelve a codificar el audio (con el vídeo
  copiado) no ocupa ninguna plaza.
- Un espectador que cambia de calidad sigue ocupando una sola plaza.
- Solo se codifica la calidad que el espectador está viendo de verdad, no
  toda la escala de calidades.

Cada flujo va por la vía más barata que haya demostrado que puede servirlo:
un acelerador medido en lo que necesita el flujo (por ejemplo, convertir HDR
a SDR, o sacar HEVC), o el procesador. El procesador y los aceleradores son
dos reservas separadas, y un flujo en la tarjeta gráfica sigue gastando un
poco de procesador para su audio y su empaquetado.

Cuando falta sitio, el servidor responde en este orden:

1. **Una calidad más baja.** Un flujo nuevo cuya calidad más alta no cabe se
   acepta en la más alta que sí cabe, y ese techo se mantiene durante todo el
   flujo.
2. **El procesador.** Cuando los aceleradores están llenos, un flujo nuevo
   pasa al procesador si este tiene sitio.
3. **Un rechazo.** Cuando ninguno de los dos tiene sitio, el reproductor
   muestra **El servidor está ocupado transcodificando otros flujos** y puede
   volver a intentarlo.

Si una codificación con la tarjeta gráfica falla en un título, ese título
pasa al procesador durante un tiempo, para todos los espectadores. La
tarjeta en sí solo se deja de usar si también falla en ella una codificación de
prueba que no depende del contenido, y se vuelve a probar más tarde:
consulta [Aceleración por hardware](../administration/hardware-acceleration.md).

Mientras alguien ve algo, el trabajo de fondo del propio servidor
(miniaturas, detección de intros, análisis de los medios) se pone en pausa,
para que la reproducción disponga de toda la máquina: consulta
[Tareas en segundo plano y carga del servidor](../operations/background-work.md).

Cuando un tipo de fuente sigue haciendo esperar a los espectadores a pesar
de todo esto, el servidor baja su calidad máxima durante un tiempo, y luego
quita el límite por sí solo. Consulta
[Supervisión y registros](../operations/monitoring.md#how-playback-actually-went).

## Lo que se codifica { #what-is-encoded }

| | Por defecto |
| --- | --- |
| Códec de vídeo | H.264. Salida HEVC cuando una ruta medida lo demuestra y el reproductor lo descodifica (la ruta de passthrough HDR, o cuando lo pide el **Códec de vídeo preferido** del perfil); AV1 cuando está activado (**Salida AV1**, solo por hardware) y el reproductor lo descodifica. |
| Resolución | La de la fuente, salvo que se aplique un límite: el máximo del reproductor, el límite de bitrate, o lo que el servidor puede codificar en tiempo real. |
| Bitrate | Sigue al de la fuente, ajustado a la resolución. **Techo de bitrate de vídeo** lo limita; **Techo de bitrate de vídeo, espectadores remotos** lo limita aún más fuera de las redes locales. |
| Audio | AAC. Consulta [Audio](audio.md). |
| Entrega | HLS, en segmentos MP4 fragmentados. |

<figure class="evey-shot" markdown="span">
  ![Los dos topes de bitrate de vídeo en Administración → Transcodificación](../images/admin-transcoding-bitrate.webp){ loading=lazy }
  <figcaption>Los dos topes de <strong>Administración → Transcodificación</strong>: para todos los espectadores, y para los espectadores fuera de las redes locales (interfaz en inglés).</figcaption>
</figure>

## La caché de transcodificación { #the-transcoding-cache }

Los segmentos codificados se guardan en disco, en el volumen `evey-cache`, y
se reutilizan cuando otro espectador, o el mismo más tarde, reproduce el
mismo título con la misma decisión. **Administración → Caché de
transcodificación** muestra su tamaño y la vacía; el **Presupuesto de caché
de transcodificación** (**Administración → Transcodificación**) le pone un límite. **Borrar todo** también
borra los subtítulos extraídos de tus archivos, guardados en
`./evey-data/subtitles`: se vuelven a extraer cuando se piden.

**Cuando el disco se llena.** El servidor mantiene un margen de espacio libre
en el disco que guarda la caché: 2 GiB por defecto, que se fija con
`EVEY_TRANSCODE_CACHE_MIN_FREE_MB` (`0` quita el margen). Por debajo, se
borran primero los segmentos más antiguos que nadie está viendo. Si eso no
libera bastante, un segmento que todavía no existe se rechaza con un plazo
tras el cual el reproductor puede volver a pedirlo, en lugar de arrancar un
codificador que fallaría por falta de espacio; los segmentos que ya están en
disco se siguen sirviendo. Los subtítulos extraídos en `./evey-data` siguen
el mismo margen: por debajo, los nuevos ya no se guardan, cada petición los
vuelve a extraer, y los administradores reciben **Caché de subtítulos
llena** (como mucho una vez por hora, también enviado a los webhooks como
`transcoding.derived_cache_full`).

!!! tip
    Vacía la caché después de una actualización que cambie la forma de
    codificar los vídeos, para que nadie reciba segmentos de la versión
    anterior. Si la vacías mientras alguien ve algo, su flujo vuelve a cargar
    una vez y luego sigue.

## Comprobar lo que pasa { #checking-what-happens }

- **Administración → Reproduciendo ahora**: cada flujo, su método pista por
  pista, y el motor que lo codifica.
- **Administración → Transcodificación → Calibración de codificación**: los
  techos medidos y los aceleradores en uso.
- **Administración → Calidad de streaming**: cómo ha ido la reproducción para
  los espectadores reales.
- **Prueba de carga** (**Administración → Transcodificación**): simula
  espectadores para medir lo que soporta el servidor. No se ejecuta mientras
  alguien esté viendo algo.

<!-- sources
disk floor: cmd/evey/wire.go (EVEY_TRANSCODE_CACHE_MIN_FREE_MB, DefaultMinFreeBytes, SetWriteFloor, SetSubtitleCacheWriteFloor, OnSubtitleCacheFloorHit) ; internal/app/transcoding/segment_cache_admit.go - AdmitWrite, tryOnDemandEvict, ErrCacheFull ; internal/adapters/driving/http/transcode_vod_admission.go (503 + Retry-After) ; internal/domain/transcoding/events.go - DerivedCacheFull ; i18n notifications.cache_full
clear all empties extracted subtitles: internal/app/transcoding/service.go - WithDerivedCachePurge ; internal/adapters/driving/http/subtitle_cache.go - SubtitleCachePurger (/data/subtitles)
calibration: internal/app/transcoding/calibration_service.go - EnsureCalibrated, Recalibrate, idleProbes, idleCPUPercent ; internal/domain/transcoding/capability.go - SafeFallback ; internal/adapters/driven/ffmpeg/benchmark.go - swBenchLadderSpecs (floors 720/480), sample order (path > url+sha256 > synthetic) ; docs/adr/0029-transcoding-routes-are-measured-not-assumed.md ; i18n player.calibrating, admin.calibration.contendedLine, transcodesActiveToast
capacity: i18n admin.transcoding.livePlaybackTitle, maxVideoSessionsDesc, videoSessionsAutoAdjusts ; internal/app/transcoding/admission_shaper.go - VideoAdmissionShaper, StreamPlan ; internal/app/transcoding/video_session_limiter.go - VideoSessionLimiter (PoolCPU, PoolGPU) ; internal/app/transcoding/resolved_route.go - SpillPoolFull, SpillTitleBlocked ; internal/app/transcoding/hw_route_blocklist.go ; CalibrationService.DemoteCapability
busy: i18n errors max_concurrent_reached
output codec: internal/app/transcoding/resolver_route.go ; internal/app/transcoding/decision.go ; internal/app/transcoding/testdata/route_golden.txt ; i18n admin.calibration.av1Label, av1Hint, maxBitrateLabel, maxBitrateRemoteLabel
single variant: internal/config/config.go (EVEY_TRANSCODING_SINGLE_VARIANT, default on)
bench samples: internal/config/config.go (EVEY_BENCH_{SDR,HDR}_SAMPLE{,_URL,_SHA256})
cache budget, clear while live: i18n admin.serverSettings.transcodeCacheMaxGB, admin.transcodeCache.clearConfirmLiveSessions ; automatic limits: admin.qoe.overrides.*
load test: i18n admin.loadtest.idleWarning, errorActiveStreams
plain opening, busy message: i18n errors max_concurrent_reached ; internal/app/transcoding/admission_shaper.go - VideoAdmissionShaper.GrantRoute
-->
