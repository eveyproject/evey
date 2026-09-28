---
source_sha: ec58362c8a7e791e617d5615c213693bfbd23e8c8494c6d1e5b6bce5ce563cb2
---
# Instalación

Evey se ejecuta como **un solo contenedor**: la imagen sirve la API y la
interfaz web en un único puerto y ejecuta Postgres y Meilisearch dentro de sí
misma, como el usuario del servidor sin root, con secretos generados en el
primer arranque. El archivo compose de la sección 1 es toda la instalación. No
necesita `.env`, ni `--privileged`, ni ningún ajuste de memoria o CPU.

!!! warning "No es el docker-compose.yml del repositorio"

    El `docker-compose.yml` de la raíz del repositorio Git **no** es ese archivo.
    Es la disposición del operador (sección 7): Postgres como servicio hermano, un
    Caddy incluido y un `.env` sin el que no arranca. Copie el bloque de abajo, no
    el archivo del repositorio.

Para ejecutar Evey sin Docker, vea [deployment-bare-metal.md](deployment-bare-metal.md).

## Requisitos previos

- Docker + Docker Compose v2 (`docker compose`, no el antiguo `docker-compose`).
- Un sitio donde guardar los datos en un disco persistente (el compose de abajo
  usa montajes bind en el directorio de trabajo).
- **Recomendado:** un proxy inverso para HTTPS - Evey solo sirve HTTP simple. Vea
  [reverse-proxy.md](reverse-proxy.md).

## 1. El archivo compose

Guarde esto como `docker-compose.yml`, con la ruta de sus medios en la última
línea de volúmenes. Los secretos aparecen en `./evey-data/keys` en el primer
arranque; no hay nada que inventar.

```yaml title="docker-compose.yml"
services:
  evey:
    image: gravityz/evey:latest # (1)!
    container_name: evey-server
    ports:
      - "9091:9091" # (2)!
    # environment:
    #   EVEY_SERVER_BASE_URL: http://<your-ip>:9091
    volumes:
      - ./evey-data:/data # (3)!
      - evey-cache:/cache # (4)!
      - /path/to/your/media:/media # (5)!
    stop_grace_period: 60s # (6)!
    restart: unless-stopped

volumes:
  evey-cache:
```

1. La imagen. Fije aquí una etiqueta (`0.2.0-beta.1`, o un `sha-1a2b3c4` inmutable) para que dos pulls con un día de diferencia ejecuten el mismo código - vea [Actualizar Evey](#4-updating-evey).
2. La API y la interfaz web, en un solo puerto, en HTTP simple. Para HTTPS ponga un [proxy inverso](reverse-proxy.md) delante y descomente `EVEY_SERVER_BASE_URL` justo debajo con la dirección pública: los enlaces de los correos, las redirecciones OIDC y las URL de cast se construyen a partir de ella.
3. Todo lo que pertenece a Evey: base de datos, clave JWT, secretos generados, índice de búsqueda, registros, copias de seguridad, extensiones. Es la carpeta que hay que respaldar.
4. Caché de transcodificación y de trickplay. Se puede vaciar sin riesgo: se vuelve a llenar cuando hace falta.
5. Su biblioteca, a la izquierda de los dos puntos. En lectura-escritura a propósito: la reparación del índice de búsqueda temporal reconstruye los archivos en su sitio. Añada `:ro` para una biblioteca de solo lectura y acepte que esas reparaciones fallen. Más carpetas se añaden como `/media2`, `/media3`...
6. Con `docker stop`, la base de datos integrada se detiene limpiamente después del servidor; un catálogo grande necesita más que los 10 s que Docker concede por defecto.

!!! tip "Un entorno mínimo"

    Solo los secretos que usted aporta, la conexión a la base de datos cuando es
    externa, las rutas de almacenamiento y lo relativo al proxy inverso tienen su
    sitio en el compose (o en un `.env` para los secretos). Casi todo lo demás -
    nombre del servidor, transcodificaciones simultáneas, límites de peticiones,
    nivel de registro, intervalos de escaneo y de vigilancia, calendario de copias
    de seguridad, claves de API - **se modifica en caliente desde Admin →
    Configuración del servidor** y se queda fuera del compose.

!!! info "A quién pertenecen los archivos: PUID y PGID"

    Sin definir por defecto: el entrypoint adopta el propietario de sus montajes
    `/media`, para que las reparaciones en el sitio puedan escribir. Para elegir,
    defina `PUID` y `PGID` bajo `environment:` (sus `id -u` e `id -g`). El
    contenedor arranca como root solo para aplicar esa correspondencia y después
    renuncia a sus privilegios - el servidor en sí nunca se ejecuta como root.

???+ note "En un NAS (Synology Container Manager, QNAP Container Station)"

    - Cree la carpeta de datos **antes** del primer arranque (`mkdir evey-data` junto
      al compose, o desde File Station): esas interfaces rechazan un montaje bind
      cuya carpeta no existe, a diferencia de la línea de órdenes de Docker. No hay
      que crear nada más de antemano - el punto de entrada crea él mismo `plugins/`,
      `keys/` y `postgres/`.
    - Fije `PUID`/`PGID` en el usuario DSM propietario de los medios (`id <user>` por
      SSH; en Synology suele rondar `1026`, con el grupo `100` = `users`) en lugar de
      fiarse de la detección automática: una carpeta compartida de DSM suele
      pertenecer a `root`, que el punto de entrada nunca adopta.
    - NAS Intel con Quick Sync: añada las líneas `devices:` y `group_add:` de
      `docker-compose.gpu.yml` (el GID del grupo render se obtiene con
      `getent group render` o `stat -c %g /dev/dri/renderD128`).
    - No hace falta ningún `.env` - el `docker-compose.yml` de la raíz del
      repositorio, que exige un `.env`, es la disposición del operador, no esta.

## 2. Primer arranque

```bash
docker compose up -d
docker compose logs -f evey
```

En el primer arranque, el servidor:

1. **Genera la clave de firma JWT** automáticamente en
   `EVEY_JWT_PRIVATE_KEY_PATH` (`/data/jwt.key`, dentro de `./evey-data`). No la
   crea usted a mano. Guarde bien `evey-data` - perder la clave cierra la sesión
   de todo el mundo.
2. **Ejecuta las migraciones de la base de datos automáticamente** (integradas,
   aplicadas al arrancar; sin paso de migración aparte). Vea
   [runbooks/migration-runbook.md](runbooks/migration-runbook.md).

Abra luego `http://<su-ip>:9091`. Mientras no haya administrador, llegará al
**asistente de instalación** (`/setup`): cree la primera cuenta de
administrador. Después inicie sesión y **añada una biblioteca** (Admin →
Bibliotecas) apuntando a una ruta bajo `/media` (la carpeta que montó allí). El primer escaneo empieza solo. El asistente, las
bibliotecas, invitar a la gente y lo que conviene comprobar antes de compartir la
dirección se recorren en los [primeros pasos](guide/first-steps.md).

## 3. Variantes habituales

**Variantes, cada una unas pocas líneas en ese archivo:**

- *Una segunda carpeta de medios*: añada `- /other/path:/media2` bajo `volumes:`
  (`/media3`, ... igual). Móntelas en lectura-escritura, como la
  primera.
- *Detrás de un proxy inverso que ya tiene* (Nginx Proxy Manager, Traefik, ... en
  una red Docker): quite `ports:`, únase a la red del proxy e indique al servidor
  su URL pública y en qué vecino confiar:
  ```yaml
  services:
    evey:
      environment:
        EVEY_SERVER_BASE_URL: https://media.example.com
        EVEY_TRUSTED_PROXIES: 172.16.0.0/12     # narrow to the proxy's subnet once known
  networks:
    default:
      name: proxy-net        # the network your proxy is on
      external: true
  ```
  El proxy apunta entonces a `evey-server:9091`. Ajustes por proxy:
  [reverse-proxy.md](reverse-proxy.md).
- *HTTPS sin proxy todavía*: añada el servicio Caddy incluido de
  [reverse-proxy.md](reverse-proxy.md) (certificado Let's Encrypt automático).
- *Postgres o Meilisearch como servicios propios*: sección 7.

**HTTPS y proxys inversos.** Evey sirve HTTP simple en el 9091. Sin proxy, el
servicio Caddy incluido de [reverse-proxy.md](reverse-proxy.md) le consigue un
certificado Let's Encrypt (los puertos 80/443 deben llegar al host). Con su
propio proxy (Nginx, Traefik, Nginx Proxy Manager): fije `EVEY_SERVER_BASE_URL`
en su URL pública `https://` (ese esquema es lo que activa las cookies Secure y
HSTS) y confíe en el proxy con `EVEY_TRUSTED_PROXIES`. Los endpoints de
streaming necesitan el búfer desactivado y ningún tiempo de espera - las
configuraciones listas para usar se encargan de ello. Montajes completos:
[reverse-proxy.md](reverse-proxy.md).

## 4. Actualizar Evey

```bash
docker compose pull && docker compose up -d
docker compose logs -f evey      # until "healthy" (a migration on a large catalogue takes minutes)
```

No hace falta nada más. La nueva imagen aplica las migraciones pendientes al
arrancar y antes vuelca la base de datos (`./evey-data/backups/pre-migration-*`),
la comprobación de salud del contenedor se pone en verde en cuanto sirve, y
`up -d` solo reinicia el servicio `evey` - nunca toca una base de datos o un proxy
hermanos.

**Volver atrás.** Ponga la etiqueta anterior en la línea `image:` (o en
`EVEY_VERSION` si su compose la interpola) y vuelva a ejecutar
`docker compose up -d`. Si la versión más nueva migró el esquema, restaure antes
su volcado previo a la migración - `docker exec evey-server evey admin rollback`
imprime los pasos exactos, con la ruta del volcado.

**Comprobar qué se ejecuta.** `docker exec evey-server evey version` imprime la
versión y el commit del binario en marcha (`/healthz` informa de lo mismo), así
que «¿qué código es este?» nunca depende de una etiqueta.

> Los operadores que trabajan desde una copia del repositorio tienen el mismo
> procedimiento en scripts: `scripts/evey-update.sh` (ensayo por defecto; estado,
> volcado previo a la actualización, pull, espera de salud, comprobación del
> commit, línea de vuelta atrás) y `scripts/evey-rollback.sh`. Son comodidades
> para los despliegues propios del repositorio, no algo que necesite una
> instalación.

**Fije lo que ejecuta.** Ponga en la línea `image:` una etiqueta de versión
(`0.2.0-beta.1`) o una etiqueta inmutable `sha-<short>`; una etiqueta de rama como
`beta-0.0.1` se mueve con cada push, así que dos pulls con un día de diferencia
pueden ejecutar código distinto y «volver a `beta-0.0.1`» no lleva a ninguna
parte. El script imprime la etiqueta que lanzó para que pueda fijarla.

**Etiquetas de imagen** (`gravityz/evey`):

- Las etiquetas de versión `vX.Y.Z` publican `X.Y.Z`, `X.Y`, `X` y `latest`.
  `latest` siempre significa la **versión** semver más reciente.
- Los builds de rama publican el nombre de la rama (p. ej. `beta-0.0.1`) más
  `sha-<short>`.
- **Variantes de ejecución** - la misma aplicación, con distintos entornos de
  extensiones incluidos:
  - sin sufijo (por defecto / «lean») - solo Python 3.
  - `-nodejs` - añade Node para las extensiones en JS.
  - `-full` - añade además PHP + Ruby + Lua.
  Elija `-nodejs` / `-full` solo si sus extensiones necesitan esos entornos; si no,
  la variante por defecto es la más pequeña. Fije p. ej.
  `EVEY_VERSION=latest-nodejs`.
- **Variantes de GPU** - las etiquetas publicadas incluyen todas las familias de
  aceleradores (Intel Gen9 hasta hoy, AMD), unos 900 MB solo de compiladores
  OpenCL de Intel. Una imagen construida por usted puede quedarse solo con la
  familia de su host con `docker build --build-arg GPU=intel-legacy .` (Intel Gen9
  a Gen11, las UHD 6xx), `GPU=intel` (Gen12 y posteriores), `GPU=amd` o
  `GPU=none`. La etiqueta de imagen `org.evey.gpu` guarda la elección. NVIDIA no
  necesita variante: ninguna imagen lleva bibliotecas NVIDIA integradas, el
  NVIDIA Container Toolkit inyecta el controlador del host en tiempo de ejecución
  (`--gpus all` o una reserva de dispositivo en compose), así que NVENC funciona
  con todas las variantes, `none` incluida. En una máquina con una iGPU Intel y
  una tarjeta NVIDIA, el modo `auto` de la calibración elige primero VAAPI; fije
  el modo de hardware en `nvenc` en el panel de transcodificación de la
  administración para usar la tarjeta.

## 5. Copias de seguridad y recuperación

Active las copias programadas de la base de datos (`EVEY_BACKUP_INTERVAL_HOURS`,
guardadas en `EVEY_BACKUP_DIR` → `/data/backups`, dentro de `evey-data`) o lance
una desde el panel de administración. Las copias cubren todos los datos de
usuario, no sus archivos multimedia. Detalles: [backups.md](backups.md).

Para situaciones de bloqueo, administrador perdido o restauración, la imagen
incluye una CLI `evey admin` (`reset-password`, `rotate-jwt-key`,
`restore-backup`): [runbooks/recovery.md](runbooks/recovery.md). Seguridad de las
migraciones: [runbooks/migration-runbook.md](runbooks/migration-runbook.md).

## 6. Resolución de problemas

Primeras comprobaciones cuando algo va mal:

```bash
docker compose ps                       # is every service Up / healthy?
docker compose logs --tail=200 evey     # server logs (migration errors, config validation)
```

- **evey no arranca / error de configuración.** Con una base de datos externa, el
  servidor se niega a arrancar si `EVEY_DB_PASSWORD` sigue siendo el valor por
  defecto `evey`; con un Meilisearch externo, si `MEILI_MASTER_KEY` está vacío o es
  un marcador (una clave corta distinta de la por defecto solo avisa). Lea el
  mensaje de validación registrado y corrija el `.env`.
- **La base de datos integrada no arranca.** Las líneas `evey:` al principio del
  log dicen por qué: un clúster de otra versión mayor (vea la sección de
  actualización más arriba), un directorio de datos que no es un clúster de
  Postgres, o un error con el prefijo `postgres:` del propio Postgres (disco
  lleno, permisos sobre `./evey-data/postgres`, un sistema de archivos en carpeta
  de red).
- **Bucle de reinicios con `permission denied` sobre `/cache/...` en el log.** El
  volumen `evey-cache` pertenece a otro uid distinto del que ejecuta el servidor
  (`PUID`, o el propietario de los medios adoptado). Las imágenes anteriores al
  2026-09-17 solo lo reasignaban si `./evey-data` también cambiaba de
  propietario, así que una carpeta de datos creada de antemano como `PUID` (el
  caso NAS de arriba) dejaba atrás la caché. Actualice la imagen; con una
  antigua, ejecute una vez `docker run --rm -v evey_evey-cache:/c alpine chown -R
  <PUID>:<PGID> /c` (el volumen se llama `<project>_evey-cache`).
- **La base de datos externa no responde.** Confirme que Postgres está sano y
  que la contraseña coincide:
  ```bash
  docker exec evey-postgres pg_isready -U "$EVEY_DB_USER"
  ```
- **Búsqueda rota / vacía.** Revise la salud de Meilisearch y lance una
  reindexación - desde los ajustes de búsqueda de la administración, o con
  `docker exec evey-server evey admin reindex-search`. El índice se reconstruye a
  partir de Postgres, así que borrar `meili-data` no tiene riesgo.
- **Endpoint de salud:** `curl -sf http://localhost:9091/healthz && echo OK`.

### Qué volúmenes se pueden borrar sin riesgo

| Volumen | Contenido | ¿Se puede borrar? |
|--------|----------|-----------------|
| `evey-data` | Base de datos (`postgres/`), clave JWT + secretos generados (`keys/`), logs, copias, extensiones, índice de búsqueda (`meili/`) | No - se pierden los datos, la clave JWT y las extensiones. Solo `meili/`: sí, se reconstruye con `evey admin reindex-search` |
| `postgres-data` | Base de datos (solo disposición externa) | No - se pierden todos los datos |
| `evey-cache` | Caché de transcodificación + trickplay | Sí - se regenera bajo demanda |

## 7. Disposición del operador: Postgres y Meilisearch externos

**Cómo se comporta la base de datos integrada.** El clúster vive en
`./evey-data/postgres` (Postgres 17, `initdb` en el primer arranque, propiedad del
usuario del servidor). Solo escucha en el loopback del contenedor y exige la
contraseña generada por TCP, así que una extensión rebajada a otro uid no puede
abrirlo. `docker stop` le llega como un apagado rápido y ordenado después de que
el servidor haya cerrado sus conexiones. El directorio de datos tiene que estar
en un sistema de archivos de verdad (ext4, btrfs, xfs, ...) - no en una carpeta
CIFS/NFS. Las copias (`evey admin`, el panel de administración, el `pg_dump`
programado) funcionan exactamente igual que con una base de datos externa. Para
abrir una consola SQL:

```bash
docker exec -it evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

### Postgres externo

Los operadores que quieren llevar ellos mismos el ciclo de vida de la base de
datos (un Postgres gestionado, una versión mayor fijada, un volumen aparte, volcado
y restauración a su manera) lo ejecutan como servicio hermano: añada el servicio y
apunte el servidor a él - **fijar `EVEY_DB_HOST` es lo que desactiva la base de
datos integrada**, y por eso toda instalación anterior a la base integrada sigue
funcionando sin tocar nada:

=== "docker-compose.yml"

    ```yaml
    services:
      evey:
        # ... as above, plus:
        environment:
          EVEY_DB_HOST: postgres
          EVEY_DB_SSLMODE: disable          # internal network, no TLS
        depends_on:
          postgres: { condition: service_healthy }

      postgres:
        image: postgres:16-alpine
        container_name: evey-postgres
        environment:
          POSTGRES_DB: ${EVEY_DB_NAME:-evey}
          POSTGRES_USER: ${EVEY_DB_USER:-evey}
          POSTGRES_PASSWORD: ${EVEY_DB_PASSWORD:?EVEY_DB_PASSWORD must be set in .env}
        volumes:
          - ./postgres-data:/var/lib/postgresql/data
        healthcheck:
          test: ["CMD-SHELL", "pg_isready -U ${EVEY_DB_USER:-evey}"]
          interval: 5s
          timeout: 3s
          retries: 5
        restart: unless-stopped
    ```

=== ".env"

    ```env
    EVEY_DB_PASSWORD=<strong-password>   # must NOT be the default 'evey' - the server refuses to start otherwise
    ```

**Meilisearch externo, la misma idea.** Un contenedor de búsqueda aparte conserva
su propio ciclo de vida y su volumen (y le permite hacer
`docker compose restart meilisearch` para reiniciar solo el índice). Desactive el
integrado, apunte el servidor al externo y comparta la clave - un
`MEILI_MASTER_KEY` explícito siempre gana al `./evey-data/keys/meili.key`
generado:

=== "docker-compose.yml"

    ```yaml
    services:
      evey:
        # ... plus:
        environment:
          EVEY_EMBEDDED_MEILI: "false"
          EVEY_MEILI_URL: http://meilisearch:7700
        depends_on:
          meilisearch: { condition: service_healthy }

      meilisearch:
        image: getmeili/meilisearch:v1.12
        container_name: evey-meilisearch
        environment:
          MEILI_MASTER_KEY: ${MEILI_MASTER_KEY:?MEILI_MASTER_KEY must be set in .env}
          MEILI_NO_ANALYTICS: "true"
        volumes:
          - ./meili-data:/meili_data
        healthcheck:
          test: ["CMD", "curl", "-sf", "http://localhost:7700/health"]
          interval: 5s
          retries: 5
        restart: unless-stopped
    ```

=== ".env"

    ```env
    MEILI_MASTER_KEY=<run: openssl rand -hex 16>
    ```

Dele a ese contenedor un `user:` sin root y un `mem_limit` si puede - la imagen
oficial se ejecuta como root y su memoria residente (índice mapeado en memoria)
parece grande pero es sobre todo caché de páginas recuperable. El índice se
reconstruye a partir de Postgres (`evey admin reindex-search`), así que
`meili-data` es prescindible en cualquier caso.

Solo estas variables de entorno son de infraestructura/secretas y **van en el
compose/.env** - nunca se pueden editar en la administración: `EVEY_DB_PASSWORD`,
`MEILI_MASTER_KEY`, `EVEY_DB_HOST`, `EVEY_DB_USER`, `EVEY_DB_NAME`,
`EVEY_DB_SSLMODE`, `EVEY_EMBEDDED_POSTGRES`, `EVEY_EMBEDDED_MEILI`,
`EVEY_MEILI_URL`, las rutas `*_DIR`, `EVEY_JWT_PRIVATE_KEY_PATH`,
`EVEY_SERVER_BASE_URL`, `EVEY_TRUSTED_PROXIES` y el interruptor general de las
extensiones `EVEY_PLUGINS_ENABLED`. (`EVEY_PLUGINS_SANDBOX_MODE`,
`EVEY_PLUGINS_UID_SEPARATION` y los límites de recursos pueden fijarse aquí como
valores iniciales, pero se editan en caliente desde Admin → Plugins.) Referencia
completa: [configuration.md](configuration.md).

### Pasar una instalación existente a la base de datos integrada

Una instalación que ejecuta Postgres como servicio hermano (la disposición de
todas las versiones anteriores a la base integrada) puede pasar al contenedor
único: los datos están a un volcado de distancia, y todo lo demás en
`./evey-data` (clave JWT, extensiones, copias, trickplay) se recupera tal cual. La
base integrada importa un volcado que encuentre en
`./evey-data/postgres-import.sql.gz` **una vez, en el clúster que crea en su
primer arranque**, antes de que se ejecute el servidor - así las migraciones
propias del servidor se aplican después encima, como tras cualquier
actualización.

```bash
# 1. dump the current database (plain SQL, gzipped - what the import expects)
docker compose exec -T postgres pg_dump -U evey --no-owner --no-privileges evey \
  | gzip > evey-data/postgres-import.sql.gz
docker compose down

# 2. write the new compose (keep a copy of the old one): the single-service
#    file from section 1, with your variations (extra media folders, the
#    proxy block) and whatever you had in `environment:` (hardware
#    transcoding: `devices: /dev/dri`, `group_add`, LIBVA_DRIVER_NAME).

# 3. first boot: initdb, import, migrations, then the server
docker compose up -d && docker compose logs -f evey     # "evey: import done"

# 4. search index: an external Meilisearch's index is not carried over, rebuild it
docker exec evey-server evey admin reindex-search
```

Lo que YA NO hace falta: `EVEY_DB_PASSWORD`, `MEILI_MASTER_KEY` y el cableado
`EVEY_DB_HOST` / `EVEY_MEILI_URL` (los secretos se generan), los servicios
`postgres` y `meilisearch`, y las claves API sembradas desde `.env` (ya están en
el `server_config` volcado). Conserve `./postgres-data` y `./meili-data` hasta
haber comprobado el servidor; el compose antiguo los sigue ejecutando si necesita
volver atrás. Un volcado dejado junto a un clúster ya inicializado se señala y se
ignora, nunca se fusiona; una importación fallida descarta el nuevo clúster para
que el siguiente arranque lo vuelva a intentar desde el volcado.

### Cambio de versión mayor de Postgres (base de datos integrada)

La imagen fija la versión mayor del Postgres integrado (17 hoy). Una imagen más
nueva con una versión mayor más nueva **se niega a abrir** el clúster antiguo con
un mensaje que menciona esta sección - nunca ejecuta en silencio un directorio de
datos que no sabe leer. El paso es un volcado con la imagen antigua y una
restauración con la nueva:

```bash
docker compose exec evey sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) pg_dump -h 127.0.0.1 -U evey -Fc evey' > evey-before-upgrade.dump
docker compose down
mv evey-data/postgres evey-data/postgres.old      # keep it until the restore is verified
# set the new EVEY_VERSION in .env, then:
docker compose up -d                              # initialises an empty cluster on the new major
docker compose exec -T evey sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) pg_restore -h 127.0.0.1 -U evey -d evey --no-owner' < evey-before-upgrade.dump
docker compose restart evey
```

Los operadores que prefieren `pg_upgrade` o una versión mayor fijada durante años
usan la [disposición con Postgres externo](#7-operator-layout-external-postgres-and-meilisearch).

## 8. Hosts ARM (x86-64 y ARM64)

Las versiones estables (etiquetas `vX.Y.Z`) publican una imagen
**multiarquitectura** para `linux/amd64` y `linux/arm64`; `docker pull`/`compose up`
elige la adecuada automáticamente, sin ningún indicador. Las etiquetas de versiones
previas (`-alpha`/`-beta`) y los builds por rama son **solo amd64** - fije una
`vX.Y.Z` estable en `EVEY_VERSION` para un host ARM (una Raspberry Pi, un NAS ARM,
un VPS Ampere).

Dos cosas que conviene saber en ARM:

- **Transcodificación por hardware en ARM: solo Rockchip, y solo lo que la
  calibración demuestra.** Evey conecta cuatro backends de aceleración - Intel QSV,
  VAAPI, NVENC y Rockchip **RKMPP** (SoC de la clase RK3588 / RK3576 / RK3568, con
  los codificadores `h264_rkmpp` / `hevc_rkmpp` del jellyfin-ffmpeg arm64, la
  descodificación `drm_prime`, el escalado RGA y un mapeo de tono OpenCL sobre el
  ICD Mali cuando existe). QSV y los controladores VAAPI de Intel son solo x86 (no
  se instalan a propósito en la imagen arm64); NVENC es NVIDIA. Para usar RKMPP,
  pase al contenedor el nodo del servicio MPP y el nodo de render DRM
  (`/dev/mpp_service`, `/dev/dri`, y `/dev/rga` si el núcleo lo expone); Evey lo
  detecta, hace una codificación de prueba real y luego mide y prueba cada formato
  en su placa antes de ofrecerlo - no se deduce nada del nombre del SoC, y un
  formato que no se demuestra se queda en la CPU. El proyecto aún no ha validado
  RKMPP en hardware real: considérelo en la medida de lo posible y mire en
  **Admin → Transcodificación → Calibración de codificación** qué se ha
  certificado. El codificador **V4L2 M2M** de la Raspberry Pi **no** está
  conectado: en una Pi o un NAS ARM de la clase Synology (Realtek) no hay vía de
  codificación por hardware, así que ARM significa **reproducción directa** (el
  cliente reproduce el archivo tal cual, a coste casi nulo) más
  **transcodificación por software** en la CPU. La capacidad se calibra, no se
  adivina: un núcleo débil mantiene el suelo de software de 720p y admite una
  codificación cada vez; aun así puede bajar `transcoding.max_video_sessions`
  (Admin → Transcodificación) y apoyarse en la reproducción directa.
- **El mapeo de tono HDR también va por la vía rápida en ARM.** El jellyfin-ffmpeg
  arm64 incluido trae el filtro SIMD `tonemapx` (NEON), igual que el build amd64
  trae su variante AVX2 - así que el mapeo de tono HDR→SDR es SIMD por hardware,
  no la lenta cadena zscale. (Evey sondea `tonemapx` en ejecución y solo recae en
  zscale si algún build no lo tuviera.)

Si construye usted la imagen para una arquitectura no publicada,
`docker buildx build --platform linux/arm64` funciona sin más - el Dockerfile
protege las partes solo x86 tras `TARGETARCH`.

## 9. Entorno aislado de las extensiones

Las extensiones son código de terceros que se ejecuta en procesos separados. La
imagen oficial contiene todo lo necesario para confinarlas - sin `--privileged`,
sin indicador de seguridad de Docker adicional - y
`EVEY_PLUGINS_SANDBOX_MODE=auto` (el valor por defecto) elige el backend más
fuerte que permite el host, Landlock en cualquier núcleo moderno. El propio
servidor nunca se ejecuta como root. El detalle capa por capa (seccomp,
separación de UID, límites de recursos) está en [SECURITY.md](SECURITY.md); esta
página solo le dice qué comprobar.

Tras el primer arranque, compruebe el aislamiento realmente en vigor en
**Admin → Plugins → Security Dashboard** (tarjeta del entorno aislado del host):
`auto` debería mostrar `landlock` en un núcleo moderno; `basic` significa que el
host no puede aislar el acceso de las extensiones al sistema de archivos - vea la
guía de despliegue de abajo.

Qué configuración para su máquina (NAS antiguo, sin contenedor, qué activar o desactivar, los riesgos en palabras sencillas): [plugins/sandbox-deployment.md](plugins/sandbox-deployment.md). Modelo de amenazas completo y detalle por capa: [plugins/security-model.md](plugins/security-model.md).
