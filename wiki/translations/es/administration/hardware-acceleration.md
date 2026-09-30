---
source_sha: c5dbea370fd3437769623f35f8bb5f32be76bca382bacfd248fac739d7bb12e9
reviewed: false
---
# Aceleración por hardware

Una tarjeta gráfica o una GPU integrada permite al servidor transcodificar más
flujos a la vez, y a mayor resolución, que el procesador solo. Hay una imagen
para todos los casos: no incluye ningún controlador para una tarjeta concreta, y
el mismo contenedor funciona solo con software, con una GPU Intel o AMD, o con
una tarjeta NVIDIA, según los dispositivos que le des.

Dos pasos: exponer el dispositivo al contenedor y dejar **Aceleración por
hardware** en **Auto (mejor disponible)** en **Admin → Transcodificación →
Calibración de codificación**.

## Exponer el dispositivo

=== "Intel / AMD (VAAPI, Quick Sync)"

    ```yaml title="docker-compose.yml"
    services:
      evey:
        devices:
          - /dev/dri:/dev/dri
        group_add:
          - "104"   # el grupo render del equipo: getent group render
    ```

    El grupo debe ser el grupo `render` del equipo (`getent group render`, o
    `stat -c %g /dev/dri/renderD128`). Un valor erróneo no rompe nada:
    simplemente la GPU no es accesible y el servidor se queda en el procesador.

=== "NVIDIA (NVENC)"

    Instala el NVIDIA Container Toolkit en el equipo, reserva la GPU y pide la
    capacidad `video`, sin la cual no se inyecta la biblioteca del codificador:

    ```yaml title="docker-compose.yml"
    services:
      evey:
        environment:
          NVIDIA_DRIVER_CAPABILITIES: all   # debe incluir "video"
        deploy:
          resources:
            reservations:
              devices:
                - driver: nvidia
                  count: all
                  capabilities: [gpu]
    ```

=== "Rockchip (RKMPP)"

    Pasa `/dev/mpp_service`, `/dev/dri` y, cuando el kernel lo expone,
    `/dev/rga`. Consulta [Equipos ARM](../operations/advanced-deployments.md#arm-hosts).

Se pueden pasar a la vez una reserva NVIDIA y `/dev/dri`.

## Qué hace Auto

Con **Auto**, el servidor trabaja solo a partir de mediciones, nunca del nombre de
la tarjeta:

1. **Detectar.** Cada nodo de render se prueba con VAAPI y, en Intel, con Quick
   Sync; igual cada GPU NVIDIA y el codificador de Rockchip. Un codificador solo
   cuenta si una codificación real tiene éxito.
2. **Certificar.** Cada acelerador codifica la escala de resoluciones (de 720p a
   2160p) con la misma forma de canalización que una reproducción real, conserva
   el escalón más alto que sostiene a 1,2 veces el tiempo real y pasa una
   comprobación visual. Un acelerador que no sostiene 720p no se usa.
3. **Medir las vías.** Para cada capacidad que el acelerador demostró (SDR, HEVC,
   AV1, mapeo de tonos en la GPU, HDR conservado) se mide el coste de una sesión
   real.
4. **Elegir por flujo.** Cada reproducción va al acelerador más barato que haya
   demostrado lo que ese flujo necesita, o al procesador. Consulta
   [Transcodificación y capacidad del servidor](../playback/transcoding.md).

La medición se ejecuta en el primer momento tranquilo del servidor y de nuevo
cuando cambia el hardware. **Recalibrar** la ejecuta de inmediato: úsalo tras
añadir una tarjeta o mover el contenedor a otra máquina.

**Desactivada (solo software)** mantiene toda transcodificación en el
procesador, la salida de emergencia ante un controlador defectuoso. Elegir un
**Backend** a mano (NVENC, VAAPI, QSV, RKMPP) lo fuerza; si resulta inutilizable,
el servidor vuelve al procesador y la tarjeta muestra el backend realmente en
uso.

En una máquina con varios aceleradores, Auto no tiene preferencia fija: cada uno
se certifica y se mide, y cada flujo va al backend que demostró la vía que
necesita, con la mejor calidad demostrada y el menor coste medido. Dos flujos del
mismo servidor pueden usar así backends distintos, por ejemplo el mapeo de tonos
HDR en una tarjeta y una codificación HEVC en otra.

## Comprobar que se usa

**Admin → Transcodificación → Calibración de codificación** muestra los
aceleradores detectados, el backend en uso, las resoluciones certificadas para la
GPU y para el procesador, y cualquier vuelta al procesador. Durante la
reproducción, **Admin → Reproduciendo ahora** indica el backend con el que se
codifica cada flujo.

En WSL2, NVENC funciona; VAAPI y Quick Sync no (el controlador gráfico de WSL
falla en esas codificaciones), y el panel de administración los muestra en gris.
