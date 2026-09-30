---
source_sha: c8d03e544227da6967bbbab98675cdb62fcd32e683e410544a6fffc79d40c1d5
reviewed: false
---
# Transcodificación y capacidad del servidor

La transcodificación es el resultado caro de [la decisión de reproducción](how-playback-is-decided.md):
la Reproducción directa y el Remux no le cuestan casi nada al servidor y nunca
ocupan una plaza de transcodificación. Esta página explica cómo sabe el servidor
lo que puede codificar, cómo reparte esa capacidad entre los espectadores y qué
pasa cuando está lleno.

## El servidor se mide a sí mismo

La capacidad no es un ajuste que tengas que estimar. El servidor la mide:

- **Cuándo.** No al arrancar: el servidor arranca con lo que sabía antes (o con
  un valor por defecto prudente de 720p por software la primera vez) y ejecuta la
  medición en su primer momento tranquilo, cuando nadie está viendo nada y el
  procesador está casi inactivo. Tarda unos minutos. Una medición hecha con carga
  se marca como provisional y se repite cuando la máquina está inactiva.
- **Qué.** Codificaciones reales de un clip 4K, en SDR y en HDR, en el procesador
  y en cada acelerador que el servidor detectó, con la misma forma que una
  reproducción real. El resultado es la resolución más alta que cada uno sostiene
  en tiempo real y el coste de cada tipo de flujo.
- **De nuevo.** Cuando cambia el hardware, o con **Recalibrar** en **Admin →
  Transcodificación → Calibración de codificación**.

Mientras se ejecuta la medición, una reproducción que necesita transcodificar
muestra **El servidor se está calibrando** y se le pide que lo intente de nuevo
en breve. La Reproducción directa no se ve afectada.

En un procesador que no sostiene ninguna de las resoluciones, el servidor sigue
ofreciendo 720p (SDR) y 480p (HDR), un flujo a la vez.

La medición usa, por este orden, un clip que proporciones tú, un clip de
referencia descargado una vez y verificado por su SHA256, o un clip sintético
generado en el momento. Un clip real mide con más honestidad que el sintético,
que exagera el coste y puede limitar una máquina por debajo de lo que merece:

```yaml title="docker-compose.yml"
services:
  evey:
    environment:
      EVEY_BENCH_HDR_SAMPLE: /data/bench/my-4k-hdr.hevc
      EVEY_BENCH_SDR_SAMPLE: /data/bench/my-4k-sdr.hevc
```

## Repartir la capacidad

**Capacidad de reproducción en directo** (**Admin → Transcodificación**) es el
número de flujos de vídeo que se pueden codificar a la vez. Se deduce de la
medición y se ajusta sola cuando cambia el hardware. **Limitar las emisiones de
vídeo simultáneas** puede bajarla, nunca subirla.

- Una transcodificación que solo recodifica el audio (vídeo copiado) no ocupa
  ninguna plaza.
- Un espectador que cambia de calidad conserva una sola plaza.
- Solo se codifica la calidad que el espectador ve realmente, no toda la escala
  de calidades.

Cada flujo va a la vía más barata que pueda servirlo: un acelerador que haya
demostrado lo que el flujo necesita (por ejemplo el mapeo de tonos HDR, o una
salida HEVC), o el procesador. El procesador y los aceleradores son dos reservas
separadas. Cuando los aceleradores están llenos, un nuevo flujo puede pasar al
procesador si tiene sitio; cuando ninguno tiene sitio, el reproductor muestra
**El servidor está ocupado transcodificando otros flujos** y puede reintentarlo.

Mientras alguien ve algo, las tareas propias del servidor en segundo plano
(miniaturas, detección de intros, análisis de medios) se pausan, para que la
reproducción conserve la máquina: consulta
[Tareas en segundo plano y carga del servidor](../operations/background-work.md).

Cuando un tipo de fuente sigue haciendo esperar a los espectadores pese a todo
esto, el servidor baja su calidad máxima durante un tiempo y luego levanta el
límite por sí solo. Consulta
[Supervisión y registros](../operations/monitoring.md#how-playback-actually-went).

## Qué se codifica

| | Por defecto |
| --- | --- |
| Códec de vídeo | H.264. HEVC cuando el reproductor lo descodifica y la tarjeta gráfica lo codifica; AV1 cuando está activado (**Salida AV1**, solo por hardware) y el reproductor lo descodifica. |
| Resolución | La de la fuente, salvo que se aplique un límite: el máximo del reproductor, el límite de bitrate o lo que el servidor puede codificar en tiempo real. |
| Bitrate | Sigue a la fuente, escalado a la resolución. **Tope de bitrate de vídeo** lo limita. |
| Audio | AAC. Consulta [Audio](audio.md). |
| Entrega | HLS, en segmentos MP4 fragmentados. |

<figure class="evey-shot" markdown="span">
  ![Los dos topes de bitrate de vídeo en Admin → Transcodificación](../images/admin-transcoding-bitrate.webp){ loading=lazy }
  <figcaption>Los dos topes de <strong>Admin → Transcodificación</strong>: para todos los espectadores y para los espectadores fuera de las redes locales (interfaz en inglés).</figcaption>
</figure>

## La caché de transcodificación

Los segmentos codificados se guardan en disco en el volumen `evey-cache` y se
reutilizan cuando otro espectador, o el mismo más tarde, reproduce el mismo
título con la misma decisión. **Admin → Caché de transcodificación** muestra su
tamaño y la vacía; el **Presupuesto de caché de transcodificación** (**Admin →
Configuración del servidor → Configuración del servidor**) la limita. Vacíala
tras una actualización que cambie cómo se codifican los vídeos, para que a nadie
se le sirvan segmentos de la versión anterior.

## Comprobar lo que pasa

- **Admin → Reproduciendo ahora**: cada flujo, su método pista por pista y el
  backend que lo codifica.
- **Admin → Transcodificación → Calibración de codificación**: los techos medidos
  y los aceleradores en uso.
- **Admin → Calidad de streaming**: cómo fue la reproducción para los
  espectadores reales.
- **Prueba de carga** (**Admin → Transcodificación**): simula espectadores para
  medir lo que sostiene el servidor. Ejecútala solo cuando nadie esté viendo nada.
