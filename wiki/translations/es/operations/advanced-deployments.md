---
source_sha: 6c5be1c7b73eceb506748f84e685e858d94021c34fe6715f0d4127dfd10d261f
reviewed: false
---
# Despliegues avanzados

La [instalación en un solo contenedor](../installation.md) sirve para la mayoría
de los servidores. Esta página cubre las disposiciones que puede preferir quien
administra el servidor: la base de datos o el motor de búsqueda como servicios
aparte, los equipos ARM, las variantes de la imagen y el sandbox de plugins en
equipos poco habituales. Para ejecutar Evey sin Docker, consulta
[Sin contenedor](../deployment-bare-metal.md).

## La base de datos integrada

El clúster de la base de datos vive en `./evey-data/postgres` (Postgres 17,
creado en el primer arranque, propiedad del usuario del servidor). Solo escucha
en el loopback del contenedor y exige la contraseña generada, así que un plugin
que se ejecuta con otro usuario no puede abrirla. `docker stop` la apaga
limpiamente después de que el servidor haya cerrado sus conexiones.

El directorio de datos debe estar en un sistema de archivos local (ext4, btrfs,
xfs...), no en un recurso compartido SMB/CIFS o NFS.

Para abrir un shell SQL:

```bash
docker exec -it evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

### Actualización mayor de Postgres

La imagen fija la versión mayor del Postgres integrado (17). Una imagen con una
versión mayor más reciente se niega a abrir el clúster antiguo, con un mensaje
que remite aquí, en lugar de ejecutar un directorio de datos que no puede leer.
El camino es un volcado con la imagen antigua y una restauración con la nueva:

```bash
docker compose exec evey sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) pg_dump -h 127.0.0.1 -U evey -Fc evey' > evey-before-upgrade.dump
docker compose down
mv evey-data/postgres evey-data/postgres.old      # consérvalo hasta verificar la restauración
# pon la nueva etiqueta de imagen en el archivo compose y después:
docker compose up -d                              # crea un clúster vacío con la nueva versión mayor
docker compose exec -T evey sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) pg_restore -h 127.0.0.1 -U evey -d evey --no-owner' < evey-before-upgrade.dump
docker compose restart evey
```

## Postgres externo

Para gestionar tú mismo el ciclo de vida de la base de datos (un Postgres
gestionado, una versión mayor fijada, tu propia política de volcados), ejecútala
como un servicio aparte y apunta el servidor a ella. **Definir `EVEY_DB_HOST` es
lo que desactiva la base de datos integrada.**

=== "docker-compose.yml"

    ```yaml
    services:
      evey:
        # ... como en el archivo de contenedor único, más:
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
    EVEY_DB_PASSWORD=<strong-password>   # el servidor se niega a arrancar con el valor por defecto 'evey'
    ```

## Meilisearch externo

El mismo principio para el motor de búsqueda: desactiva el integrado, apunta el
servidor al tuyo y comparte la clave. Un `MEILI_MASTER_KEY` explícito prevalece
sobre el `./evey-data/keys/meili.key` generado.

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

El índice se reconstruye a partir de la base de datos (`evey admin reindex-search`),
así que `meili-data` siempre se puede descartar.

## Pasar a la base de datos integrada

Una instalación que ejecuta Postgres como servicio aparte puede pasar al
contenedor único. La base de datos integrada importa una vez un volcado que
encuentra en `./evey-data/postgres-import.sql.gz`, en el clúster que crea en su
primer arranque, antes de que arranque el servidor; las migraciones del servidor
se aplican después encima.

```bash
# 1. vuelca la base de datos actual (SQL plano, comprimido con gzip)
docker compose exec -T postgres pg_dump -U evey --no-owner --no-privileges evey \
  | gzip > evey-data/postgres-import.sql.gz
docker compose down

# 2. sustituye el archivo compose por el de contenedor único, conservando tus
#    carpetas de medios, los ajustes del proxy inverso y las líneas de hardware.

# 3. primer arranque: crear el clúster, importar, migrar, arrancar el servidor
docker compose up -d && docker compose logs -f evey     # "evey: import done"

# 4. el índice de un Meilisearch externo no se traslada: reconstrúyelo
docker exec evey-server evey admin reindex-search
```

`EVEY_DB_PASSWORD`, `MEILI_MASTER_KEY` y los ajustes `EVEY_DB_HOST` /
`EVEY_MEILI_URL` ya no son necesarios. Conserva `./postgres-data` hasta haber
comprobado el servidor. Un volcado dejado junto a un clúster existente se indica
y se ignora, nunca se fusiona; una importación fallida descarta el clúster nuevo
para que el siguiente arranque lo reintente.

## Equipos ARM

Las imágenes construidas desde una etiqueta de versión (`0.2.0`, incluidas las
versiones previas como `0.2.0-beta.1`) son multiarquitectura (`linux/amd64` y
`linux/arm64`); Docker elige la correcta. Las etiquetas de rama como
`beta-0.0.1` son solo amd64: fija una etiqueta de versión en un equipo ARM.

- **La transcodificación por hardware en ARM** se limita a los SoC Rockchip
  (clase RK3588, RK3576, RK3568) mediante RKMPP. Pasa `/dev/mpp_service`,
  `/dev/dri` y, cuando el kernel lo expone, `/dev/rga`; Evey sondea y mide el
  codificador antes de usarlo. El proyecto aún no ha validado RKMPP en hardware
  real. Una Raspberry Pi o un NAS ARM basado en Realtek no tiene ningún
  codificador por hardware que Evey pueda usar: esos servidores se apoyan en la
  Reproducción directa y en la transcodificación por software, dimensionadas por
  la misma calibración que cualquier otro equipo.
- **El mapeo de tonos HDR** usa en ARM (NEON) el mismo filtro SIMD `tonemapx` que
  en x86-64 (AVX2).

## Etiquetas de imagen

La imagen es `gravityz/evey`.

- Las etiquetas de versión `vX.Y.Z` publican `X.Y.Z`, `X.Y`, `X` y `latest`.
  `latest` es siempre la versión estable más reciente: una versión previa
  (`vX.Y.Z-beta.N`) solo publica su propia etiqueta y nunca la mueve.
- Las compilaciones de rama publican el nombre de la rama (por ejemplo
  `beta-0.0.1`) y `sha-<commit>`.
- **Entornos de ejecución de plugins.** La imagen por defecto incluye Python 3
  para los plugins. La variante `-nodejs` añade Node.js; `-full` añade PHP, Ruby
  y Lua. Usa una variante solo si un plugin que instalas la necesita.
- **Familias de GPU.** Las imágenes publicadas llevan los controladores de todas
  las generaciones Intel y AMD compatibles. Una imagen compilada por ti puede
  conservar solo una con `docker build --build-arg GPU=intel-legacy .` (Intel Gen9
  a Gen11), `GPU=intel` (Gen12 y posteriores), `GPU=amd` o `GPU=none`. NVIDIA no
  necesita variante: el NVIDIA Container Toolkit inyecta el controlador del
  equipo al ejecutar.

## Sandbox de plugins

La imagen contiene todo lo necesario para confinar los plugins, sin modo
privilegiado. El modo de sandbox por defecto (`auto`) elige el backend más fuerte
que permite el equipo: Landlock en cualquier kernel moderno. Comprueba el modo
realmente activo en el **Panel de seguridad** de **Admin → Plugins**. Un equipo
que muestra `basic` no puede aislar el acceso de los plugins a los archivos:
consulta [Configurar el sandbox de plugins](../plugins/sandbox-deployment.md)
para saber qué significa y qué cambiar. Las capas en sí se describen en
[Modelo de seguridad y sandbox de plugins](../SECURITY.md).
