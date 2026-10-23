---
source_sha: 458502112f637df70456890a718f35fac167120f02775459773b334b689e454f
reviewed: true
---
# Despliegues avanzados { #advanced-deployments }

La [instalación en un solo contenedor](../installation.md) sirve para la
mayoría de los servidores. Abre esta página si quieres la base de datos o el
motor de búsqueda como servicios aparte, si el equipo es ARM, si necesitas
una variante de la imagen, o si el sandbox de plugins indica algo distinto de
`landlock`. Para usar Evey sin Docker, consulta
[Sin contenedor](../deployment-bare-metal.md).

## La base de datos integrada { #the-embedded-database }

El clúster está en `./evey-data/postgres` (Postgres 17, creado en el primer
arranque y propiedad del usuario del servidor). Solo escucha en la interfaz
local del contenedor y exige la contraseña generada, así que un plugin que se
ejecuta con otro usuario no puede abrirlo. `docker stop` lo apaga limpiamente
después de que el servidor haya cerrado sus conexiones.

La carpeta de datos tiene que estar en un sistema de archivos local (ext4,
btrfs, xfs...), no en una carpeta compartida SMB/CIFS o NFS. Para abrir una
consola SQL:

```bash
docker exec -it evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

### Pasar a una versión mayor más reciente de Postgres { #move-to-a-newer-postgres-major }

La imagen fija la versión mayor de la base de datos integrada. Una imagen que
trae una versión mayor más reciente se niega a abrir el clúster antiguo, con
un mensaje que remite a esta sección, en lugar de usar una carpeta de datos
que no sabe leer. El camino es un volcado con la imagen antigua y una
restauración con la nueva:

```bash
docker compose exec evey sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) pg_dump -h 127.0.0.1 -U evey -Fc evey' > evey-before-upgrade.dump
docker compose down
mv evey-data/postgres evey-data/postgres.old      # consérvalo hasta comprobar la restauración
# pon la nueva etiqueta de imagen en el archivo compose y después:
docker compose up -d                              # crea un clúster vacío con la nueva versión mayor
docker compose exec -T evey sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) pg_restore -h 127.0.0.1 -U evey -d evey --no-owner' < evey-before-upgrade.dump
docker compose restart evey
```

## Usar Postgres como servicio aparte { #run-postgres-as-a-separate-service }

Para gestionar tú mismo el ciclo de vida de la base de datos (un Postgres gestionado,
una versión mayor fijada, tu propia política de volcados), ejecútala como un
servicio aparte e indícale al servidor dónde está. Definir `EVEY_DB_HOST` es
lo que desactiva la base de datos integrada. El servidor se niega a arrancar
con la contraseña por defecto `evey`.

=== "docker-compose.yml"

    ```yaml
    services:
      evey:
        # ... como en el archivo de un solo contenedor, más:
        environment:
          EVEY_DB_HOST: postgres
          EVEY_DB_SSLMODE: disable          # red interna, sin TLS
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
    EVEY_DB_PASSWORD=<strong-password>
    ```

## Usar Meilisearch como servicio aparte { #run-meilisearch-as-a-separate-service }

El mismo principio sirve para el motor de búsqueda: desactiva el integrado,
indícale al servidor dónde está el tuyo y comparte la clave. Un
`MEILI_MASTER_KEY` explícito tiene prioridad sobre la clave generada
`./evey-data/keys/meili.key`.

=== "docker-compose.yml"

    ```yaml
    services:
      evey:
        # ... más:
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

El índice de búsqueda se reconstruye a partir de la base de datos con
`docker exec evey-server evey admin reindex-search`, así que siempre puedes
borrar `meili-data`.

## Volver a la base de datos integrada { #move-back-onto-the-embedded-database }

Una instalación que usa Postgres como servicio aparte puede pasar al
contenedor único. La base de datos integrada importa una sola vez el volcado
que encuentre en `./evey-data/postgres-import.sql.gz`, dentro del clúster que
crea en su primer arranque y antes de que arranque el servidor. Después, las
migraciones del servidor se aplican encima.

```bash
# 1. vuelca la base de datos actual (SQL plano, comprimido con gzip)
docker compose exec -T postgres pg_dump -U evey --no-owner --no-privileges evey \
  | gzip > evey-data/postgres-import.sql.gz
docker compose down

# 2. sustituye el archivo compose por el de un solo contenedor, conservando
#    tus carpetas de medios, los ajustes del proxy inverso y las líneas de hardware.

# 3. primer arranque: crea el clúster, importa, migra y arranca el servidor
docker compose up -d && docker compose logs -f evey     # "evey: import done"

# 4. el índice de un Meilisearch externo no se traslada: reconstrúyelo
docker exec evey-server evey admin reindex-search
```

`EVEY_DB_PASSWORD`, `MEILI_MASTER_KEY` y los ajustes `EVEY_DB_HOST` /
`EVEY_MEILI_URL` ya no hacen falta. Conserva `./postgres-data` hasta haber
comprobado el servidor. Un volcado dejado junto a un clúster que ya existe se
señala y se ignora, nunca se mezcla. Si la importación falla, el clúster nuevo
se descarta para que el siguiente arranque vuelva a intentarlo desde el
volcado.

## Equipos ARM { #arm-hosts }

Las imágenes construidas a partir de una etiqueta de versión (`0.1.0`,
incluidas las versiones previas) son multiarquitectura
(`linux/amd64` y `linux/arm64`): Docker elige la adecuada. Las etiquetas de rama
son solo amd64: en un equipo ARM, fija una etiqueta de
versión.

- **La transcodificación por hardware en ARM** se limita a los chips Rockchip
  (de la clase RK3588, RK3576, RK3568) a través de RKMPP. Da acceso a
  `/dev/mpp_service`, `/dev/dri` y, si el kernel lo expone, `/dev/rga`; el
  servidor prueba y mide el codificador antes de usarlo. El proyecto todavía
  no ha validado RKMPP en hardware real. Una Raspberry Pi o un NAS ARM con
  chip Realtek no tiene ningún codificador por hardware que el servidor pueda
  usar: esos equipos dependen de la reproducción directa y de la
  transcodificación por software, dimensionadas con la misma calibración que
  cualquier otro equipo.
- **El mapeo de tonos HDR** usa el mismo filtro SIMD `tonemapx` en ARM (NEON)
  que en x86-64 (AVX2).

## Etiquetas y variantes de la imagen { #image-tags-and-variants }

La imagen es `eveyproject/evey`.

| Etiqueta | Publicada por | Notas |
|---|---|---|
| `X.Y.Z`, `X.Y`, `X`, `latest` | una etiqueta de versión `vX.Y.Z` | `latest` es siempre la versión estable más reciente |
| una etiqueta de versión previa (con guion) | una etiqueta de versión previa | solo publica su propia etiqueta, nunca mueve `latest` |
| un nombre de rama, `sha-<commit>` | una compilación de rama | solo amd64 |

Cada etiqueta existe en tres variantes: la variante por defecto incluye
Python 3 para los plugins, `-nodejs` añade Node.js y `-full` añade PHP, Ruby y
Lua (por ejemplo `latest-full`). Usa una variante solo si un plugin la
necesita: el instalador rechaza un plugin cuyo lenguaje declarado falta en la
imagen, con un mensaje que indica la variante correcta.

Las imágenes publicadas llevan los controladores de todas las generaciones
Intel y AMD compatibles. Una imagen que construyas tú puede quedarse con una
sola familia con `docker build --build-arg GPU=intel-legacy .` (Intel de Gen9
a Gen11), `GPU=intel` (Gen12 y posteriores), `GPU=amd` o `GPU=none`. NVIDIA no
necesita variante: el NVIDIA Container Toolkit inyecta el controlador del
equipo al ejecutar el contenedor.

## El sandbox de plugins en un equipo poco habitual { #the-plugin-sandbox-on-an-unusual-host }

La imagen contiene todo lo necesario para confinar los plugins, sin modo
privilegiado. El modo de sandbox por defecto (`auto`) elige el mecanismo más
fuerte que permite el equipo: Landlock en cualquier kernel moderno. Comprueba
el modo activo en el **Panel de seguridad** de **Administración → Plugins**.
Un equipo que muestra `basic` no puede aislar el acceso de los plugins a los
archivos: [Configurar el sandbox de plugins](../plugins/sandbox-deployment.md)
explica lo que eso significa y qué cambiar. Las capas en sí se describen en
[Sandbox de plugins y modelo de amenazas](../SECURITY.md).

<!-- sources
embedded db, import, major upgrade, embedded meili: docker/entrypoint.sh (PG_MAJOR, PG_IMPORT, "evey: import done", EVEY_EMBEDDED_MEILI, MEILI_MASTER_KEY_FILE)
external db switch and default password refusal: internal/config/config.go (EVEY_DB_HOST, hardFail on "evey")
reindex: cmd/evey/admin.go - adminReindexSearch
arm, variants, GPU families: Dockerfile (TARGETARCH, RUNTIMES, GPU) ; .github/workflows/ci.yml (tags, flavor suffix, platforms) ; internal/adapters/driven/ffmpeg/hwdetect.go - detectRKMPP
sandbox auto: internal/adapters/driven/pluginrunner/runner_landlock.go - resolveAutoMode ; i18n/en/admin.yaml - plugins.security.title
-->
