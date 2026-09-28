---
source_sha: 0de496a664ed813f5ad423017ddfe0563349e3baaee95acf2ee582c2e970dd84
---

# Transcodificación

Evey transcodifica al vuelo cuando el cliente no admite el formato de origen.

**En resumen.** La mayoría de las instalaciones no necesitan nada aquí: el
servidor mide lo que la máquina puede convertir y admite ese número de
conversiones a la vez. Para usar una tarjeta gráfica, expóngala al contenedor
(más abajo) y deje **Aceleración por hardware** en **Auto** en **Admin →
Transcodificación**. El resto de la página explica lo que hay detrás de esa
elección.

## Cómo funciona

Evey elige por sí solo el mejor método de reproducción:
1. **Reproducción directa** - el formato está admitido, no se procesa nada
2. **Remux** - se reempaqueta el contenedor (MKV → MP4), sin recodificar
3. **Transcodificación** - se recodifica a HLS con tasa de bits adaptativa

## Configuración

Los dos valores de abajo solo siembran el primer arranque: el panel de
administración (**Admin → Transcodificación**) manda en cuanto se guarda un
valor allí.

```env
EVEY_TRANSCODING_MAX_CONCURRENT=2
# auto | off | nvenc | vaapi | qsv | rkmpp - véase «Aceleración por hardware», más abajo
EVEY_TRANSCODING_HW_MODE=off
```

## Aceleración por hardware

Evey distribuye **una sola imagen**, cuyo ffmpeg lleva compilados NVENC, VAAPI y
QSV a la vez. En la imagen no va grabado ningún controlador de GPU: el
controlador lo aporta usted en tiempo de ejecución, con solo asignar el
dispositivo, y sin necesidad de modo privilegiado. Esa misma imagen es solo
software, NVENC o VAAPI/QSV según lo que le asigne y nada más. El razonamiento
está en [ADR-0015](adr/0015-hardware-acceleration.md).

### NVIDIA (NVENC)

Instale el [NVIDIA Container Toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/install-guide.html) y reserve después la GPU **y** pida la capacidad `video` del controlador (sin ella, el toolkit inyecta `nvidia-smi` y CUDA, pero no `libnvidia-encode.so`, y NVENC no llega a cargarse):

```yaml
services:
  evey:
    environment:
      NVIDIA_DRIVER_CAPABILITIES: all   # tiene que incluir `video` para NVENC
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: all
              capabilities: [gpu]
```

### Intel / AMD (VAAPI, QSV)

Exponga el nodo de renderizado DRM; libva escoge sola el controlador adecuado
(iHD, i965 o radeonsi) a partir del id PCI de la GPU:

```yaml
services:
  evey:
    devices:
      - /dev/dri:/dev/dri
```

Puede asignar los dos a la vez (la reserva de NVIDIA más `/dev/dri`): la imagen
es una sola y sirve para el backend que después seleccione.

### Cómo activarla

La aceleración por hardware se maneja desde **Admin → Transcodificación →
Calibración de codificación**:

- Un interruptor general de **aceleración por hardware**. Desactivado = solo
  software (la válvula de escape para una GPU o un controlador caprichosos);
  activado = se usa la GPU.
- Con él activado, aparece un selector de **Backend**: `Auto` (la calibración
  elige el mejor de los detectados) o forzar `NVENC`, `VAAPI`, `QSV` o `RKMPP`. Si el
  backend forzado resulta inservible, se recae en software y la tarjeta muestra
  cuál está activo de verdad.

**Qué hace `Auto`.** Usted expone el hardware al contenedor (`/dev/dri`, la
reserva de NVIDIA, el nodo MPP de Rockchip); Evey hace el resto, solo a partir
de mediciones - nunca por el nombre de una CPU o una GPU:

1. **Detectar**: se sondea cada nodo de render DRM para VAAPI y, en un nodo
   Intel, para QSV; cada GPU NVIDIA por su índice CUDA; el nodo MPP de
   Rockchip. Cada codificador tiene que superar una codificación real antes de
   contar.
2. **Certificar**: cada acelerador detectado recorre la escalera de
   codificación (720p → 2160p) con la misma forma de pipeline que usa la
   ejecución, se queda con el peldaño más alto que sostiene a 1,2× tiempo real
   y luego pasa un control de corrección visual. Por debajo del tiempo real a
   720p = rechazado, solo software. Cada backend certificado se guarda con **su
   propio** techo, matriz de descodificación, funciones y rutas probadas - un
   acelerador que solo llega a 1080p junto a otro que llega a 2160p sirve
   escaleras de 1080p, nunca las del otro.
3. **Medir las rutas**: para cada ruta que un backend ha probado (SDR, HEVC,
   AV1, mapeo de tono en GPU, puente por CPU, HDR conservado, descodificación
   desacoplada) y para las rutas por software, se mide una sesión de producción
   en caliente en los peldaños de referencia y en el techo de la ruta - así un
   backend certificado solo en 720p, o el suelo de software de 720p, también
   tiene su coste.
4. **Elegir según la carga**: el resolutor elige, en cada petición, el backend
   certificado más barato que haya probado la ruta y admita el códec (el
   HDR→SDR puede ir al mapeo de tono OpenCL de VAAPI mientras el HEVC toma un
   QSV más rápido en la misma máquina). La admisión tasa la sesión según la
   medición de esa ruta frente a una reserva de CPU y otra de acelerador; una
   reserva llena desvía un flujo de GPU a la CPU o rechaza.

La escalera por software tiene un suelo de **720p SDR / 480p HDR**: una CPU
que no supera ningún peldaño con margen sigue ofreciendo esos peldaños,
admitiendo una sola sesión a la vez. La reproducción directa y la copia/remux
nunca ocupan una plaza de codificación y no se ven afectadas por nada de esto.

La elección se guarda y se vuelve a medir en caliente, sin reiniciar. También se
puede sembrar desde el entorno, para los despliegues sin interfaz:

```env
# O bien el antiguo interruptor de sí/no…
EVEY_TRANSCODING_HW_ACCEL_ENABLED=true
# …o bien el modo unificado (auto|off|nvenc|vaapi|qsv|rkmpp); el valor del panel de
# administración manda sobre este.
EVEY_TRANSCODING_HW_MODE=auto
# Pasar a software sin esperar más cuando una GPU se queda atascada (5 s por defecto).
EVEY_TRANSCODING_HW_FIRST_SEGMENT_TIMEOUT=5s
# Lanzar un ffmpeg por peldaño ABR en la vía de hardware (un codificador por
# peldaño) en lugar de un único proceso de varias salidas: así se aíslan los
# fallos de cada peldaño y se evita el grafo de codificación múltiple que hace
# caer a los controladores VA más frágiles. Desactivado por defecto; la vía de
# software siempre usa el grafo de un solo proceso. Véase docs/adr/0015.
EVEY_TRANSCODING_HW_PER_VARIANT=false
# Codificar únicamente el peldaño ABR que el cliente esté pidiendo de verdad
# (bajo demanda, un codificador por peldaño), en lugar de un ffmpeg que produce
# la escalera entera.
# ACTIVADO por defecto (=1). La multivariante comparte una descodificación entre
# los peldaños, pero también codifica cada peldaño que el espectador nunca ve -
# medido en producción, tres codificaciones inútiles ahogaban la cuarta. La
# calibración mide la MISMA forma que ejecuta el servidor (un peldaño por sonda
# en monovariante, la escalera paralela acumulada en multivariante), para las
# escaleras por software Y por hardware, de modo que los techos corresponden a
# lo que cuesta de verdad una sesión. Ponga =0 solo en una máquina sin
# descodificación por hardware Y con fuentes pesadas, para recuperar la
# descodificación compartida. La caché funciona igual en los dos casos.
EVEY_TRANSCODING_SINGLE_VARIANT=1
```

> **Cinco ajustes viven ya en el panel de administración** (Admin >
> Transcodificación > Límites avanzados): `HW_PER_VARIANT`,
> `SINGLE_VARIANT`, `MAX_SESSIONS`, `SOFTWARE_MAX_HEIGHT` y
> `SOFTWARE_HDR_MAX_HEIGHT`. Un valor guardado ahí se conserva y prevalece sobre
> el entorno, de modo que ya no hay que editar un archivo compose y recrear el
> contenedor para cambiar un techo de reproducción. Las cinco se leen en el
> arranque, y el panel lo dice: guardar registra su elección, reiniciar la
> aplica.

### Muestra para la calibración de la codificación

El benchmark mide contra un clip en 4K hasta dónde llega esta máquina
codificando en tiempo real, por separado para SDR y para HDR
(`software_max_height` y `software_hdr_max_height`). **El arranque nunca lo
ejecuta**: el servidor arranca en milisegundos con lo que tenga (la calibración
guardada, o un repliegue por software en 720p en el primer arranque) y lanza el
benchmark en el primer *momento tranquilo* - cinco sondeos seguidos, separados
4 s, sin ninguna reproducción y con la CPU por debajo del 20 %. Así un
benchmark de 2 a 5 minutos queda fuera del arranque y lejos de una máquina
ocupada con una migración o un escaneo de biblioteca (un benchmark medido bajo
carga se marca como *provisional* y se repite en reposo). Mientras corre, a una
reproducción nueva se le pide esperar unos 2 minutos (`calibrating`); **Admin →
Transcodificación → Calibración de codificación** indica si la calibración está
pendiente, en curso o hecha, y «Recalibrar»
(`POST /api/v1/admin/transcode/calibration/recalibrate`) la lanza enseguida. El tope sale del coste de
descodificar, mapear el tono y codificar ese clip, de manera que **un clip de
contenido real calibra con mucha más honradez que uno sintético**: el sintético
exagera el coste y deja el tope demasiado bajo (síntoma clásico: una película en
4K HDR que la máquina reproduce sin problema a 1080p acaba limitada a 480p o
360p).

El benchmark resuelve su entrada por orden de prioridad:

1. **Archivo del operador** - un clip real que deja usted. Se usa tal cual.
2. **Descarga fijada** - un clip de referencia sin problemas de licencia que se descarga una sola vez (con su SHA256 verificado) y queda en caché.
3. **Sintético** - generado sobre la marcha (`testsrc2` más un poco de grano, a unos 5 Mbit/s). Siempre disponible; es el recurso para una máquina sin red.

```bash
# Apuntar el benchmark a un archivo real (suyo, o un clip de referencia descargado):
EVEY_BENCH_HDR_SAMPLE=/data/bench/my-4k-hdr.hevc
EVEY_BENCH_SDR_SAMPLE=/data/bench/my-4k-sdr.hevc
# …o a una descarga fijada (la URL tiene que ser https; el SHA256 es obligatorio cuando hay URL):
EVEY_BENCH_HDR_SAMPLE_URL=https://example.org/evey-bench-hdr10.hevc
EVEY_BENCH_HDR_SAMPLE_SHA256=<sha256>
EVEY_BENCH_SDR_SAMPLE_URL=https://example.org/evey-bench-sdr.hevc
EVEY_BENCH_SDR_SAMPLE_SHA256=<sha256>
```

Los clips de referencia se construyen a partir de una fuente de licencia
permisiva (las películas abiertas de Blender, CC-BY; **no** contenido CC-BY-NC-ND
como el «Sol Levante» de Netflix) con
`scripts/make-bench-sample.sh <source-4k>`, que además imprime el SHA256 que hay
que fijar.

### La salvedad de WSL2

En WSL2 se llega a la GPU a través del controlador experimental **d3d12** de
Mesa. **NVENC funciona** (hay paso directo real de CUDA); **VAAPI y QSV no**: el
controlador VA de d3d12 se cae con la codificación multivariante de Evey. Evey
detecta que está en WSL y atenúa VAAPI y QSV en el panel de administración.
Valide VAAPI y QSV en un host Linux nativo.

### HDR conservado y HDR10+

En una GPU sondeada para la codificación Main10 (`hw_hdr10_encode`), una fuente
HDR reproducida por un cliente compatible con HDR que descodifica HEVC se
recodifica como HEVC Main10 conservando el HDR: la señalización HDR10 estática
(bt2020 / PQ o HLG, SEI de la pantalla de masterización) se transmite. Los
metadatos dinámicos HDR10+ **no** se reemiten, así que un título HDR10+ se
reproduce como HDR10 simple por esa vía. Dolby Vision perfil 7 recae del mismo
modo en su capa base HDR10; el perfil 5 se convierte a SDR con mapeo de tono.

La frecuencia de muestreo del audio nunca es motivo para recodificar: una pista
de 96 kHz que la lista de códecs del cliente acepta se copia tal cual. Un
cliente cuyo descodificador se detiene en 48 kHz tiene que decirlo en su
declaración de códecs; el servidor no lo presupone.

## Audio AC-3 / E-AC-3 en el navegador (descodificador WebAssembly)

Casi ningún navegador sabe descodificar AC-3 ni E-AC-3 de forma nativa (es cosa
de licencias), lo que clásicamente obliga a recodificar el audio en el servidor.
Evey hace otra cosa: le entrega al navegador un descodificador en WebAssembly.
La lista de reproducción maestra pasa a ser solo de vídeo, la pista AC-3 se sirve
como una versión aparte, fuera de banda (copiada, no recodificada), y el cliente
la descodifica a PCM sincronizada con el reloj del vídeo. Cero CPU del servidor
gastada en el audio cuando la fuente se empaqueta tal cual.

- Los navegadores que SÍ descodifican AC-3 de forma nativa (los de la familia de
  Safari) quedan fuera por detección de capacidades, no por su nombre: a ellos se
  les hace el passthrough de siempre.
- **Requisito del proxy inverso**: la CSP que llegue al navegador tiene que
  incluir `'wasm-unsafe-eval'` en `script-src` o el descodificador no podrá
  compilarse y la película se verá muda. Véase [reverse-proxy.md](reverse-proxy.md#security-headers--csp---read-this-if-your-proxy-sets-its-own).

## Comprobación

Abra **Admin → Transcodificación → Calibración de codificación**: ahí
están los aceleradores detectados, el backend activo, los techos medidos por
software y por hardware y cualquier repliegue automático de hardware a software.
El botón «Recalibrar» vuelve a ejecutar el benchmark después de un cambio de
hardware o de controlador.
