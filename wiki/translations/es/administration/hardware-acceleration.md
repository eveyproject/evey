---
source_sha: 1a4e824d130bb84b068353b3024d71ea59b8db2ff04ee70f219e637e02212468
reviewed: true
---
# Aceleración por hardware

**En resumen:**

- Una tarjeta gráfica, o la parte gráfica que llevan integrada muchos
  procesadores (Intel Quick Sync, por ejemplo), permite al servidor
  convertir más vídeos a la vez, y a resoluciones más altas, que el
  procesador solo.
- Solo ayuda cuando un vídeo se convierte (se transcodifica). Las películas
  que se envían tal cual, o que solo se reempaquetan, no la usan.
- Lo que tienes que hacer: añadir unas líneas a tu archivo compose para que
  Evey pueda usar la tarjeta, y después dejar el ajuste en **Auto (mejor
  disponible)**.
- Evey prueba la tarjeta con una conversión real antes de enviarle ninguna
  reproducción. Una tarjeta que no pasa la prueba no se usa.

A continuación vienen los pasos, y después cómo comprueba el servidor lo que
puede hacer la tarjeta.

## Activarla

1. Abre **Administración → Transcodificación → Calibración de
   codificación**. **Aceleración por hardware** está en **Auto (mejor
   disponible)** por defecto, y la línea **Detectado:** nombra los
   aceleradores que anuncia la máquina.
2. Si dice **No se detectó acelerador GPU**, dale el dispositivo al
   contenedor (siguiente sección) y vuelve a crearlo.
3. Pulsa **Recalibrar**, o espera: el servidor se mide a sí mismo en su
   primer momento tranquilo. La tarjeta muestra entonces el backend en uso y
   las resoluciones certificadas para la GPU y para el procesador.

Hay una sola imagen para todos los casos: no lleva incluido ningún driver
para una tarjeta concreta, y el mismo contenedor funciona solo con software,
con una GPU Intel o AMD, o con una tarjeta NVIDIA, según los dispositivos
que le des.

## Dar el dispositivo al contenedor

=== "Intel / AMD (VAAPI, Quick Sync)"

    ```yaml title="docker-compose.yml"
    services:
      evey:
        devices:
          - /dev/dri:/dev/dri
        group_add:
          - "104"   # el grupo render de la máquina: getent group render
    ```

    El grupo tiene que ser el grupo `render` de la máquina
    (`getent group render`, o `stat -c %g /dev/dri/renderD128`). Un valor
    equivocado no rompe nada: simplemente la GPU no es accesible y el
    servidor se queda con el procesador.

=== "NVIDIA (NVENC)"

    Instala el NVIDIA Container Toolkit en la máquina, y después reserva la
    GPU y pide la capacidad `video`, sin la cual no se inyecta la biblioteca
    del codificador:

    ```yaml title="docker-compose.yml"
    services:
      evey:
        environment:
          NVIDIA_DRIVER_CAPABILITIES: all   # tiene que incluir "video"
        deploy:
          resources:
            reservations:
              devices:
                - driver: nvidia
                  count: all
                  capabilities: [gpu]
    ```

=== "Rockchip (RKMPP)"

    Conecta `/dev/mpp_service`, `/dev/dri` y, si el kernel lo expone,
    `/dev/rga`. Ver [Máquinas ARM](../operations/advanced-deployments.md#arm-hosts).

Se pueden conectar a la vez una reserva NVIDIA y `/dev/dri`.

## Qué significa "comprobado con una codificación real"

Con **Auto**, el servidor trabaja solo con mediciones, nunca con el nombre
de la tarjeta:

1. **Detectar.** En cada nodo de renderizado se prueba VAAPI y, en Intel,
   Quick Sync; lo mismo con cada GPU NVIDIA y con el codificador de
   Rockchip. Un codificador solo cuenta si una codificación real corta a
   través de él sale bien.
2. **Certificar.** Cada acelerador codifica la escala de resoluciones (de
   720p a 2160p) con la misma cadena de procesamiento que una reproducción
   real, y se queda con el escalón más alto que aguanta a 1,2 veces el
   tiempo real. Algunas imágenes de cada prueba se vuelven a decodificar y
   se comparan con el original, porque un driver defectuoso puede terminar
   sin error mientras produce imágenes verdes o deformadas. Un acelerador
   que no aguanta 720p no se usa.
3. **Medir las rutas.** Para cada capacidad que el acelerador ha demostrado
   (SDR, HEVC, AV1, mapeo de tonos en la GPU, HDR conservado), se mide el
   coste de una sesión real.
4. **Elegir para cada flujo.** Cada reproducción va al acelerador más
   barato que ha demostrado lo que necesita ese flujo, o al procesador. Ver
   [Transcodificación y capacidad del servidor](../playback/transcoding.md).

La medición espera a un momento tranquilo: ningún flujo activo y la CPU de
la máquina por debajo del 20 % en cinco muestras seguidas, tomadas cada
cuatro segundos, para que un arranque cargado o un escaneo de biblioteca no
dejen los límites demasiado bajos. Se repite cuando cambia el hardware.
**Recalibrar** la lanza en el momento: úsalo después de añadir una tarjeta o
de mover el contenedor a otra máquina.

!!! note "Un arranque en frío no es un veredicto"
    Una GPU que no consigue iniciarse en los primeros segundos después de
    arrancar el contenedor se vuelve a probar varias veces en poco tiempo
    antes de anotar nada. Solo un fallo que persiste, o uno de un tipo
    determinista (codificador ausente, formato no compatible), marca el
    acelerador como inutilizable.

## Forzar un backend o desactivarla

**Desactivada (solo software)** deja todas las transcodificaciones en el
procesador; es la salida de emergencia ante un driver defectuoso. Elegir un
**Backend** a mano (NVENC, VAAPI, QSV, RKMPP) fuerza ese; si resulta
inutilizable, el servidor vuelve al procesador y la tarjeta muestra el
backend que se usa de verdad.

En una máquina con varios aceleradores, Auto no tiene una preferencia fija:
cada uno se certifica y se mide, y cada flujo va al backend que ha
demostrado la ruta que necesita, con la mejor calidad comprobada y el menor
coste medido. Por eso dos flujos en el mismo servidor pueden usar backends
distintos, por ejemplo el mapeo de tonos HDR en una tarjeta y una
codificación HEVC en otra.

## Comprobar que se usa

**Administración → Transcodificación → Calibración de codificación** muestra
los aceleradores detectados, el backend en uso, las resoluciones
certificadas para la GPU y para el procesador y, en **Repliegues a
software**, las codificaciones recientes que tuvieron que volver al
procesador, con su causa. Durante la reproducción, **Administración →
Reproduciendo ahora** indica con qué backend se codifica cada flujo.

En WSL2, NVENC funciona; VAAPI y Quick Sync no (el driver gráfico de WSL
falla con estas codificaciones), y el panel las marca como **no compatible
en WSL**.

<!-- sources
auto, certify, routes, recalibrate: docs/adr/0015-hardware-acceleration.md ; docs/adr/0029-transcoding-routes-are-measured-not-assumed.md
detection by real encode: internal/adapters/driven/ffmpeg/hwdetect.go - probeHWEncode
ladder, 1.2x realtime margin, 720p floor: internal/adapters/driven/ffmpeg/benchmark.go - hwBenchLadder, Margin
visual check: internal/adapters/driven/ffmpeg/hw_output_validation.go
cold-boot init retry: internal/adapters/driven/ffmpeg/hw_readiness.go - runHWReadinessProbe, hwReadinessRetryDelays
quiet moment: internal/app/transcoding/calibration_service.go - idleProbes, idleProbeEvery, idleCPUPercent
render group: docker-compose.gpu.yml ; nvidia: docker-compose.gpu-nvidia.yml
panel: frontend/apps/web/src/features/admin/EncodeCalibrationCard.tsx (hw_mode, detected_backends, is_wsl)
labels: i18n admin.calibration.cardTitle, hwModeLabel, hwModeAuto, hwModeOff, hwBackendLabel, detected, detectedNone, recalibrate, hwFallbacks, wslUnsupported ; admin.nav.transcodingConfig, nowPlaying
plain opening: rules below ; i18n admin.calibration.hwModeAuto
-->
