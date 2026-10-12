---
source_sha: a631c73d13e67b6253b0aa9e7c5c362475b1e0fcb6153effecea88bd092feca5
reviewed: true
---
# HDR y Dolby Vision

**En resumen:**

- El **HDR** es una imagen con zonas brillantes más intensas y colores más
  ricos que la imagen habitual (llamada **SDR**). Dolby Vision es un tipo de
  HDR. El HDR necesita una pantalla capaz de mostrarlo.
- En una pantalla HDR con la app de Android o de Android TV (y un dispositivo
  que descodifique HEVC), recibes la imagen HDR original, siempre que nada
  más obligue a convertirla.
- En un navegador web, y en cualquier pantalla que no sea HDR, el servidor
  convierte la imagen a SDR. Es a propósito: el HDR mostrado en una pantalla
  SDR se ve gris y apagado.
- Convertir HDR a SDR es el trabajo más pesado que puede hacer el
  procesador. Una tarjeta gráfica ayuda: consulta
  [Aceleración por hardware](../administration/hardware-acceleration.md).

El resto de esta página explica cada caso: la imagen original, una nueva
codificación que conserva el HDR, o una conversión a SDR. Las reglas
generales de la reproducción están en
[Cómo elige Evey el modo de reproducción](how-playback-is-decided.md).

## Lo que verás, según el dispositivo

| Tu pantalla | Lo que llega |
| --- | --- |
| Una pantalla HDR con Android TV o un teléfono Android, con descodificador HEVC | El original HDR10 o HLG, intacto, salvo que otra cosa (bitrate, resolución, un subtítulo incrustado) obligue a volver a codificar. |
| Un navegador | Una conversión a SDR, siempre: el reproductor web no declara una pantalla HDR, así que el servidor convierte la imagen para él. |
| Un receptor de envío | Lo que el receptor ha indicado sobre su propia pantalla: consulta [Enviar a Chromecast](casting.md). |
| Una pantalla SDR, en cualquier sitio | Una conversión a SDR. |

Mostrar HDR en una pantalla SDR sin convertirlo da colores grises y
apagados, así que el servidor nunca lo envía. Las apps de Android comprueban
la pantalla cada vez que empieza una reproducción, no una sola vez al
instalarse: si conectas una tele HDR a un reproductor multimedia, el cambio
se nota en el siguiente título, cambio de pista o cambio de calidad.

## Cuándo se conserva el original

El vídeo HDR10 o HLG se envía tal cual cuando el reproductor indica que su
pantalla muestra HDR y que descodifica el códec, y ningún otro motivo
(bitrate, resolución, contenedor) obliga a volver a codificar.

## Cuándo hay que volver a codificar el vídeo de todos modos

Un límite de bitrate, un límite de resolución o un subtítulo que hay que
incrustar pueden obligar a volver a codificar una fuente HDR. Entonces el HDR
se conserva si se cumple todo esto:

- el reproductor indica una pantalla HDR y descodifica HEVC;
- el servidor tiene una tarjeta gráfica que ha demostrado, durante su
  medición, que codifica HEVC de 10 bits con HDR;
- esa tarjeta está disponible para este flujo.

El resultado es HEVC de 10 bits con la señalización HDR de la fuente (color
BT.2020, transferencia PQ o HLG). Los metadatos dinámicos de HDR10+ no se
conservan: un título HDR10+ se reproduce como HDR10 por este camino.

Cuando falla una de las condiciones, el procesador o la tarjeta convierten la
imagen a SDR (tone-mapping) antes de codificarla. Información del flujo
muestra entonces **HDR convertido a SDR (tone-mapping)**.

Si se podría haber enviado el original pero el servidor todavía no puede
servirlo como copia (por ejemplo, porque su índice de posicionamiento aún se está
construyendo), el vídeo se vuelve a codificar conservando el HDR en una
tarjeta así, e Información del flujo muestra **Copia no disponible, HDR
conservado en la recodificación**.

## Cuando la pantalla es SDR

Un reproductor que no indica una pantalla HDR recibe una imagen convertida a
SDR. La conversión se hace en la tarjeta gráfica cuando ha demostrado que
puede (VAAPI con OpenCL, por ejemplo), y si no en el procesador: con un
filtro optimizado SIMD (AVX2) cuando la versión de ffmpeg lo incluye, y con
una cadena de filtros portátil si no.

!!! note
    En el procesador, convertir HDR a SDR es la transcodificación más cara
    que existe. El servidor mide un techo de resolución aparte para el HDR,
    normalmente más bajo, así que un título HDR puede convertirse a una
    resolución más baja que uno SDR en la misma máquina. Consulta
    [Transcodificación](transcoding.md#the-server-measures-itself).

## Dolby Vision

Lo que indica un reproductor dice si su pantalla muestra HDR, no si admite
Dolby Vision, y Evey nunca deduce que admite Dolby Vision porque admita HDR:
enviar Dolby Vision a una pantalla que solo maneja HDR10 da colores
equivocados. El flujo solo se copia cuando lleva una capa base que se ve
bien sin ninguna descodificación Dolby Vision:

| Perfil Dolby Vision | Reproductor HDR | Reproductor SDR |
| --- | --- | --- |
| **8.1** (capa base compatible con HDR10) | Se trata como HDR10: se aplican las reglas de arriba, el original puede conservarse. | Convertido a SDR. |
| **7** (Blu-ray UHD) | La capa base HDR10 se vuelve a codificar en HEVC de 10 bits conservando el HDR. Información del flujo: **Capa base Dolby Vision recodificada (HDR conservado)**. | Convertido a SDR. |
| **5** (sin capa base HDR10) | Convertido a SDR. | Convertido a SDR. |

## Comprobar lo que ha pasado

**Información del flujo** muestra la decisión sobre el vídeo y, entre los
motivos, la conversión a SDR o el HDR conservado. Todos los caminos que
convierten a SDR lo indican ahí, con una excepción: cuando la tarjeta
gráfica está llena justo cuando empieza un flujo, el flujo pasa al
procesador después de calcularse la Información del flujo.
**Administración → Reproduciendo ahora** muestra el motor que se usa de
verdad.

<!-- sources
decision: internal/app/transcoding/decision.go - evaluateVideo (hdr_tonemapping_needed, dovi_base_layer, doViBaseLayerIsHDR10)
re-encode with HDR kept, copy unavailable: internal/app/transcoding/playback_plan_resolver.go - Decide (hdr_copy_unavailable) ; internal/app/transcoding/resolver_route.go - naturalRoute, passthroughWanted, hwTonemapWanted
hdr signalling carried: internal/adapters/driven/ffmpeg/hls_args_hw.go - hwHDRPassthroughArgs (no HDR10+ metadata)
dovi copy rule: internal/app/transcoding/copy_eligibility.go (dovi_profile, DoViBLCompat)
who declares an HDR screen: frontend/packages/client/src/native/displayHdr.ts - probeDisplayHdr ; frontend/packages/client/src/native/playSession.ts (supportsHdr) ; frontend/apps/web/src/cast-receiver/receiverLogic.ts (hdr=1) ; the web player sends no hdr parameter
tonemapx, software HDR ceiling: internal/adapters/driven/ffmpeg/tonemapx.go ; internal/pkg/hdr/hdr.go ; docs/adr/0017-software-hdr-tonemap-delivery-resolution.md
tests: internal/app/transcoding/route_golden_test.go ; internal/app/transcoding/route_tonemap_report_test.go
labels: i18n player.streamInfo.reason.* ; admin.nowPlaying
plain opening: rules below ; tone mapping cost: internal/app/transcoding/calibration_service.go (separate HDR ceiling)
-->
