---
source_sha: 5c15d0f69d445cc33f1a7038b71f5435492c5280093f9b5d8ce87e76a2707d53
reviewed: false
---
# HDR y Dolby Vision

Una imagen HDR puede llegar a la pantalla de tres formas: el original, intacto;
una recodificación que conserva el HDR; o una recodificación convertida a SDR
mediante mapeo de tonos. Cuál se aplica depende de la pantalla del reproductor,
del formato del HDR y de lo que el hardware del servidor haya demostrado que
puede hacer.

## Cuándo se conserva el original

El vídeo HDR10 o HLG se envía tal cual cuando el reproductor indica que su
pantalla muestra HDR y descodifica el códec, y ningún otro motivo (bitrate,
resolución, contenedor) obliga a recodificar. Las reglas generales están en
[Cómo elige Evey el modo de reproducción](how-playback-is-decided.md).

## Cuándo hay que recodificar el vídeo de todos modos

Un límite de bitrate, un límite de resolución o un subtítulo que incrustar pueden
obligar a recodificar una fuente HDR. El HDR se conserva entonces si se cumple
todo esto:

- el reproductor indica una pantalla HDR y descodifica HEVC;
- el servidor tiene una tarjeta gráfica que demostró, durante su medición, que
  codifica HEVC de 10 bits con HDR;
- esa tarjeta está disponible para este flujo.

El resultado es HEVC de 10 bits con la información HDR10 estática (espacio de
color, transferencia, pantalla de masterización) conservada. Los metadatos
dinámicos HDR10+ no se conservan: un título HDR10+ se reproduce como HDR10 por
esta vía.

Cuando falla una de las condiciones, el procesador o la tarjeta convierten la
imagen a SDR antes de codificarla. Información del flujo indica entonces
**HDR convertido a SDR (tone-mapping)**.

Si el original podría haberse enviado pero el servidor todavía no puede servirlo
como copia (por ejemplo porque su índice de búsqueda aún se está construyendo),
el vídeo se recodifica con el HDR conservado en una tarjeta así, e Información
del flujo muestra **Copia no disponible, HDR conservado en la recodificación**.

## Cuándo la pantalla es SDR

Un reproductor que no indica una pantalla HDR recibe una imagen SDR convertida
por mapeo de tonos. Mostrar HDR en una pantalla SDR sin conversión da colores
lavados y grises, así que el servidor nunca lo envía.

El mapeo de tonos se ejecuta en la tarjeta gráfica cuando ha demostrado que puede
(VAAPI con OpenCL, por ejemplo) y, si no, en el procesador con un filtro
optimizado con SIMD (AVX2 en x86-64, NEON en ARM). En el procesador es la
transcodificación más cara que existe: el servidor mide un techo de resolución
aparte, normalmente más bajo, para el HDR, así que un título HDR puede
convertirse a una resolución menor que uno SDR en la misma máquina.

## Dolby Vision

Lo que indica un reproductor dice si su pantalla muestra HDR, no si admite Dolby
Vision, y Evey nunca deduce la compatibilidad con Dolby Vision a partir de la
compatibilidad con HDR: enviar Dolby Vision a una pantalla que solo admite HDR10
da colores erróneos. El flujo solo se copia cuando lleva una capa base que se ve
correctamente sin ninguna descodificación Dolby Vision:

| Perfil Dolby Vision | Reproductor HDR | Reproductor SDR |
| --- | --- | --- |
| **8.1** (capa base compatible con HDR10) | Se trata como HDR10: se aplican las reglas de arriba y el original puede conservarse. | Convertido a SDR. |
| **7** (UHD Blu-ray) | La capa base HDR10 se recodifica en HEVC de 10 bits con el HDR conservado. Información del flujo: **Capa base Dolby Vision recodificada (HDR conservado)**. | Convertido a SDR. |
| **5** (sin capa base HDR10) | Convertido a SDR. | Convertido a SDR. |

## Comprobar lo que pasó

**Información del flujo** muestra la decisión sobre el vídeo y, entre los
motivos, el mapeo de tonos o el HDR conservado. Todas las vías que convierten a
SDR lo indican ahí, con una excepción: cuando la tarjeta gráfica está llena en el
momento en que empieza un flujo, el flujo pasa al procesador después de calcular
Información del flujo. **Admin → Reproduciendo ahora** muestra el backend
realmente usado.
