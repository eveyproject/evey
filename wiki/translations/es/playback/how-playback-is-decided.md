---
source_sha: 88ef0be48e8114ca12ceadc61b0194ba4760d654ae7cc58afdf1a52a3405c78f
reviewed: false
---
# Cómo elige Evey el modo de reproducción

Evey nunca te pide elegir una calidad antes de pulsar reproducir. Cada vez que un
reproductor inicia un título, el servidor decide cómo entregarlo a partir de tres
cosas: lo que contiene el archivo, lo que este reproductor puede descodificar y
lo que este servidor puede codificar en ese momento. La misma película puede así
enviarse intacta a un dispositivo y recodificada a otro, o reproducirse de otra
forma en el mismo dispositivo cuando el servidor está ocupado.

## Los tres resultados

| En Información del flujo | Lo que recibe el reproductor |
| --- | --- |
| **Reproducción directa** | El archivo original, byte a byte. |
| **Remux** | El vídeo y el audio originales en otro contenedor. No se recodifica nada. |
| **Transcodificación** | Al menos una pista recodificada. A menudo solo el audio, con el vídeo todavía original. |

El servidor los prefiere en ese orden y, dentro de una transcodificación, solo
recodifica las pistas que lo necesitan.

## Lo que aporta cada parte

**El archivo.** Códec, resolución, profundidad de bits, formato HDR, bitrate y
las pistas de audio y subtítulos, leídos al escanear el archivo.

**El reproductor.** Al iniciar la reproducción, la aplicación indica lo que su
dispositivo puede descodificar: las familias de códecs que admite, los
contenedores que abre, su resolución máxima, si su pantalla muestra HDR, cuántos
canales de audio emite y, en algunos dispositivos, un techo más bajo para H.264.
La aplicación web pregunta al navegador; las aplicaciones Android y Android TV
preguntan a los descodificadores y a la pantalla del dispositivo. Evey no guarda
ninguna lista de dispositivos compatibles: usa lo que indica el reproductor. Una
sesión de envío la describe el propio receptor cuando puede indicar sus
capacidades, y un perfil prudente en caso contrario: consulta
[Enviar a Chromecast](casting.md).

**La sesión.** Las preferencias del perfil (bitrate máximo, canales de audio
máximos, tratamiento de subtítulos) y los límites que se aplican a este
espectador, abajo.

**El servidor.** Si puede codificar esto en tiempo real, en su tarjeta gráfica o
en su procesador, y si hay una plaza de transcodificación libre. Se mide, no se
supone: consulta [Transcodificación y capacidad del servidor](transcoding.md).

## Un archivo, tres resultados

Una película 4K HEVC en HDR10 a 70 Mb/s, con una pista Dolby TrueHD 7.1,
reproducida en un Android TV cuyo descodificador admite HEVC y cuya pantalla
muestra HDR:

- **Sin límite de bitrate.** El televisor descodifica el vídeo y muestra el HDR,
  así que el vídeo se conserva tal cual. No descodifica TrueHD, así que solo se
  recodifica el audio. Información del flujo muestra **Transcodificación**, con el
  vídeo *copiado*.
- **Un límite de 20 Mb/s y una tarjeta gráfica que codifica HEVC de 10 bits con
  HDR.** El vídeo debe recodificarse para caber. El servidor lo recodifica en
  HEVC y conserva el HDR, porque tanto el televisor como la tarjeta pueden.
- **El mismo límite, con la tarjeta gráfica ocupada o no disponible para este
  título.** Toma el relevo el procesador. Codifica en H.264, que aquí no lleva
  HDR, así que la imagen se convierte a SDR mediante mapeo de tonos.

Ni el archivo ni el televisor cambiaron. Cambiaron el límite de la sesión y el
estado del servidor.

## Restricciones que aplica el propio servidor

Algunos límites no los indican hoy los reproductores y el servidor los aplica
como regla de compatibilidad prudente, igual para todos: el H.264 de 10 bits, y
cualquier vídeo en 4:2:2, 4:4:4 o de 12 bits, se recodifican, mientras que HEVC,
VP9 y AV1 hasta 10
bits 4:2:0 quedan a cargo del soporte de códecs del reproductor. Dolby Vision
solo se conserva en una forma que el reproductor pueda mostrar correctamente:
consulta [HDR y Dolby Vision](hdr.md).

## Límites de la sesión

Un título cuyo bitrate (vídeo más la pista de audio en reproducción) supera el
menor de estos límites se transcodifica por debajo de él:

- el máximo que indica el reproductor;
- el **Bitrate máximo** de la cuenta, fijado por un administrador;
- el **Bitrate máximo de streaming** del perfil, y su **Bitrate máximo en datos
  móviles** cuando un teléfono indica una conexión móvil;
- el **Bitrate remoto máximo** (**Admin → Acceso remoto**) y el techo para
  espectadores remotos de **Admin → Transcodificación**, el menor de los dos,
  cuando la dirección del espectador es pública y está fuera de las **Redes
  LAN**. Una dirección privada nunca se limita.

## Leer Información del flujo

**Información del flujo** (el botón de información, (i), en los controles del
reproductor; en el televisor, en el menú de opciones del reproductor) muestra el
método, después cada pista con su decisión (*copiado*, *transcodificado*, o
*descodificado en el cliente* para el audio que descodifica el propio reproductor
web) y, bajo **Por qué no hay reproducción directa**, los motivos.

<figure class="evey-shot evey-shot--narrow" markdown="span">
  ![Información del flujo en el reproductor web, para un Remux](../images/player-stream-info.webp){ loading=lazy }
  <figcaption>Un Remux: vídeo y audio copiados, y el único motivo por el que el archivo no se reproduce tal cual (interfaz en inglés).</figcaption>
</figure>

Los motivos se agrupan en cuatro familias:

| De dónde viene el motivo | Ejemplos tal como aparecen |
| --- | --- |
| El reproductor no puede usar la pista tal cual | Códec de vídeo que este reproductor no puede descodificar · Códec de audio que este reproductor no puede descodificar · Resolución por encima del límite del reproductor · Más canales de audio de los que admite este reproductor · Contenedor no reproducible tal cual |
| Un límite de esta sesión | Tasa de bits por encima del límite permitido · La línea medida por el cliente no puede llevar la copia |
| Una consecuencia para la imagen o el sonido | HDR convertido a SDR (tone-mapping) · Subtítulos incrustados en la imagen · Resincronización de audio solicitada |
| Un ajuste o una decisión ya tomada | Reproducción directa desactivada en el perfil · Remux desactivado en el perfil · Códec de vídeo preferido (ajuste del perfil) · Elección fijada para esta sesión |

Solo se juzga la pista de audio en reproducción. Un archivo cuya pista de
comentarios no seleccionada está en DTS no se transcodifica por ella.

Información del flujo se calcula al iniciar la reproducción. El único caso que no
puede prever es una tarjeta gráfica llena en el momento en que empieza el flujo:
el flujo pasa entonces al procesador, y **Admin → Reproduciendo ahora** muestra el
backend realmente usado.

## Por qué el método casi nunca cambia a mitad de película

La decisión se toma al iniciar la reproducción y se mantiene durante la sesión,
para que el reproductor nunca reciba segmentos de dos codificaciones distintas.
Cambiar la pista de audio o de subtítulos de un modo que lo requiera, o reiniciar
el título, provoca una nueva decisión.

## Forzar un método

**Ajustes → Reproducción** tiene **Permitir reproducción directa**, **Permitir
remux** y **Permitir transcodificación** por perfil. Desactivar uno hace que el
servidor se salte ese método, e Información del flujo muestra por qué. Es una
herramienta de diagnóstico, no un ajuste de calidad: una transcodificación
forzada nunca es mejor que el original.

**Calidad**, en los ajustes del reproductor, es otra cosa: elige entre las
calidades de un flujo transcodificado. **Auto** sigue la conexión; una altura
(1080p, 720p...) mantiene esa calidad; **Original** quita el límite de altura que
el reproductor pediría en otro caso.

## Para ir más lejos

- Los criterios en detalle: [HDR y Dolby Vision](hdr.md) · [Audio](audio.md) ·
  [Subtítulos](subtitles.md) · [Enviar a Chromecast](casting.md)
- Cómo reparte el servidor su capacidad: [Transcodificación y capacidad del servidor](transcoding.md)
- La implementación: [Cadena de reproducción](../playback-serving.md)
