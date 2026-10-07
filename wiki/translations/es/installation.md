---
source_sha: a6c61f9f7b1e619f46c4d5eea7ca2c2b93356f93c04681e95963303bdf287723
reviewed: false
---
# Instalación

Evey funciona como un único contenedor. La imagen sirve la API y la interfaz web
en un solo puerto y ejecuta dentro su propia base de datos (Postgres) y su motor
de búsqueda (Meilisearch), con contraseñas generadas en el primer arranque. El
archivo compose de abajo es toda la instalación: sin `.env`, sin modo
privilegiado, sin ajustes de memoria ni de CPU.

!!! warning "No es el docker-compose.yml del repositorio"

    El `docker-compose.yml` de la raíz del repositorio Git es otra disposición
    (Postgres como servicio separado, un Caddy incluido, un `.env` obligatorio).
    Usa el archivo de esta página. La otra disposición se describe en
    [Despliegues avanzados](operations/advanced-deployments.md).

## Requisitos

- Docker con Compose v2 (`docker compose`, no el antiguo `docker-compose`).
- Un disco persistente para los datos de Evey (el archivo compose de abajo los
  guarda en `./evey-data`, junto al archivo).
- Un equipo x86-64, o ARM64 con una etiqueta de versión: consulta
  [Equipos ARM](operations/advanced-deployments.md#arm-hosts).

Evey sirve HTTP sin cifrar. Para acceder desde fuera de casa por HTTPS, pon más
adelante un proxy inverso delante: consulta
[Acceso remoto y HTTPS](administration/remote-access.md).

## El archivo compose

Guárdalo como `docker-compose.yml` y sustituye la ruta de los medios en la última
línea de volúmenes.

```yaml title="docker-compose.yml"
services:
  evey:
    image: eveyproject/evey:latest # (1)!
    container_name: evey-server
    ports:
      - "9091:9091" # (2)!
    volumes:
      - ./evey-data:/data # (3)!
      - evey-cache:/cache # (4)!
      - /path/to/your/media:/media # (5)!
    stop_grace_period: 60s # (6)!
    restart: unless-stopped

volumes:
  evey-cache:
```

1. `latest` es la versión estable más reciente. Fija una etiqueta en cuanto uses Evey en
   serio, para que dos descargas con un día de diferencia ejecuten el mismo
   código: consulta [Copias de seguridad y actualizaciones](backups.md#updating-evey).
2. La API y la interfaz web, en un solo puerto, en HTTP sin cifrar.
3. Todo lo que pertenece a Evey: base de datos, clave de firma, contraseñas
   generadas, índice de búsqueda, miniaturas de la línea de tiempo, imágenes,
   registros, copias de seguridad, plugins. Es la carpeta que hay que respaldar.
4. Caché de transcodificación: segmentos, fotos y portadas redimensionadas. Se
   puede borrar en cualquier momento y se vuelve a llenar bajo demanda.
5. Tus medios. Montados con escritura a propósito: las reparaciones de
   [Salud de medios](runbooks/media-health.md) reescriben archivos en su sitio.
   Añade `:ro` para una biblioteca de solo lectura y esas reparaciones fallarán.
   Otras carpetas se montan como `/media2`, `/media3`...
6. Da tiempo a la base de datos integrada para detenerse limpiamente con
   `docker stop`. Los 10 segundos por defecto de Docker se quedan cortos para un
   catálogo grande.

## Primer arranque

```bash
docker compose up -d
docker compose logs -f evey
```

En el primer arranque el servidor genera su clave de firma y sus contraseñas en
`./evey-data`, crea la base de datos y aplica sus migraciones. No hay que
preparar nada a mano.

Después abre `http://<server-address>:9091`. Un servidor sin administrador abre
el asistente de configuración: continúa con
[Primera configuración](getting-started/first-setup.md).

## Dónde viven los datos

| Ubicación | Contiene | ¿Se puede borrar? |
| --- | --- | --- |
| `./evey-data` | Base de datos, clave de firma, contraseñas generadas, índice de búsqueda, miniaturas de la línea de tiempo, imágenes, registros, copias de seguridad, plugins | No. Perderla supone perder todas las cuentas, el historial y los ajustes. Dentro de ella solo puede borrarse `meili/`: el índice de búsqueda se reconstruye desde la base de datos con `evey admin reindex-search`. |
| Volumen `evey-cache` | Segmentos transcodificados, fotos y portadas redimensionadas | Sí. Se regenera bajo demanda. |
| Tus carpetas de medios | Tus archivos | Evey no los borra; quitar un título en Evey lo quita de la biblioteca, no del disco. Escribe ahí en tres casos: una reparación lanzada desde Salud de medios (el original se guarda como copia), subtítulos descargados de OpenSubtitles (una carpeta `.subtitles` junto a la película) y un archivo de prueba de unos segundos que comprueba que el vigilante de carpetas sigue recibiendo eventos. |

## Variantes habituales

Cada una son unas pocas líneas añadidas al mismo archivo.

**Una segunda carpeta de medios.** Añade `- /other/path:/media2` bajo `volumes:`.

**Propiedad de los archivos.** Por defecto el contenedor adopta el propietario
de tu montaje `/media`, para que las reparaciones puedan escribir. Para elegirlo
tú, define `PUID` y `PGID` bajo `environment:` (la salida de `id -u` y `id -g`).
El contenedor arranca como root solo para aplicar esa correspondencia; el
servidor nunca se ejecuta como root.

**Una tarjeta gráfica para transcodificar.** Pasa el dispositivo al contenedor:
consulta [Aceleración por hardware](administration/hardware-acceleration.md).

**Detrás de un proxy inverso que ya tienes.** Consulta
[Acceso remoto y HTTPS](administration/remote-access.md).

??? note "En un NAS (Synology Container Manager, QNAP Container Station)"

    - Crea la carpeta `evey-data` antes del primer arranque. Estas interfaces
      rechazan un montaje cuya carpeta no existe, a diferencia de la línea de
      comandos de Docker. No hace falta crear nada más.
    - Define `PUID` y `PGID` con el usuario del NAS propietario de los medios
      (`id <user>` por SSH; en Synology suele rondar `1026`, grupo `100`). Una
      carpeta compartida de un NAS suele pertenecer a `root`, que la detección
      automática nunca adopta.
    - NAS Intel con Quick Sync: añade las líneas `devices:` y `group_add:`
      descritas en [Aceleración por hardware](administration/hardware-acceleration.md).

## Lo que no necesitas configurar

La capacidad de transcodificación, la tarjeta gráfica, el modo de reproducción
de cada dispositivo y el acceso remoto los decide el servidor: mide la máquina
en su primer momento tranquilo, detecta el hardware que le pasaste y abre el
acceso remoto cuando le das una dirección pública. Los ajustes que pueden
cambiar más adelante (nombre del servidor, programaciones, límites, claves de
API) están en el panel de administración, no en el archivo compose. Las
variables de entorno que existen se listan en la
[Referencia de configuración](configuration.md).
