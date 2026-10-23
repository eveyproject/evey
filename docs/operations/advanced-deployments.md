# Advanced deployments

The [single-container install](../installation.md) suits most servers. Open
this page when you want the database or the search engine as separate
services, when the host is ARM, when you need an image variant, or when the
plugin sandbox reports something other than `landlock`. To run Evey without
Docker at all, see [Without a container](../deployment-bare-metal.md).

## The embedded database

The cluster lives in `./evey-data/postgres` (Postgres 17, created on the
first start, owned by the server user). It listens on the container's
loopback only and requires the generated password, so a plugin running under
another user cannot open it. `docker stop` shuts it down cleanly after the
server has closed its connections.

The data directory must be on a local filesystem (ext4, btrfs, xfs...), not
on an SMB/CIFS or NFS share. To open a SQL shell:

```bash
docker exec -it evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

### Move to a newer Postgres major

The image pins the embedded major version. An image shipping a newer major
refuses to open the old cluster, with a message pointing here, rather than
run a data directory it cannot read. The way across is a dump with the old
image and a restore with the new one:

```bash
docker compose exec evey sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) pg_dump -h 127.0.0.1 -U evey -Fc evey' > evey-before-upgrade.dump
docker compose down
mv evey-data/postgres evey-data/postgres.old      # keep it until the restore is verified
# put the new image tag in the compose file, then:
docker compose up -d                              # creates an empty cluster on the new major
docker compose exec -T evey sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) pg_restore -h 127.0.0.1 -U evey -d evey --no-owner' < evey-before-upgrade.dump
docker compose restart evey
```

## Run Postgres as a separate service

To manage the database lifecycle yourself (a managed Postgres, a pinned
major, your own dump policy), run it as its own service and point the server
at it. Setting `EVEY_DB_HOST` is what turns the embedded database off. The
server refuses to start with the default password `evey`.

=== "docker-compose.yml"

    ```yaml
    services:
      evey:
        # ... as in the single-container file, plus:
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
    EVEY_DB_PASSWORD=<strong-password>
    ```

## Run Meilisearch as a separate service

Same principle for the search engine: turn the embedded one off, point the
server at yours and share the key. An explicit `MEILI_MASTER_KEY` wins over
the generated `./evey-data/keys/meili.key`.

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

The index is rebuilt from the database with `docker exec evey-server evey
admin reindex-search`, so `meili-data` can always be discarded.

## Move back onto the embedded database

An installation that runs Postgres as a separate service can move to the
single container. The embedded database imports a dump found at
`./evey-data/postgres-import.sql.gz` once, into the cluster it creates on its
first start, before the server runs; the server's migrations then apply on
top.

```bash
# 1. dump the current database (plain SQL, gzipped)
docker compose exec -T postgres pg_dump -U evey --no-owner --no-privileges evey \
  | gzip > evey-data/postgres-import.sql.gz
docker compose down

# 2. replace the compose file with the single-container one, keeping your
#    media folders, reverse-proxy settings and hardware lines.

# 3. first start: create the cluster, import, migrate, start the server
docker compose up -d && docker compose logs -f evey     # "evey: import done"

# 4. an external Meilisearch index is not carried over: rebuild it
docker exec evey-server evey admin reindex-search
```

`EVEY_DB_PASSWORD`, `MEILI_MASTER_KEY` and the `EVEY_DB_HOST` /
`EVEY_MEILI_URL` settings are no longer needed. Keep `./postgres-data` until
you have checked the server. A dump left next to an existing cluster is
reported and ignored, never merged; a failed import discards the new cluster
so that the next start retries from the dump.

## ARM hosts

Images built from a release tag (`0.1.0`, pre-releases
included) are multi-architecture (`linux/amd64` and
`linux/arm64`); Docker picks the right one. Branch tags
are amd64 only: pin a release tag on an ARM host.

- **Hardware transcoding on ARM** is limited to Rockchip SoCs (RK3588,
  RK3576, RK3568 class) through RKMPP. Map `/dev/mpp_service`, `/dev/dri`
  and, where the kernel exposes it, `/dev/rga`; the server probes and
  benchmarks the encoder before using it. RKMPP has not been validated by
  the project on real hardware yet. A Raspberry Pi or a Realtek-based ARM
  NAS has no hardware encoder the server can use: those hosts rely on direct
  play and software transcoding, sized by the same calibration as any other
  host.
- **HDR tone mapping** uses the same SIMD `tonemapx` filter on ARM (NEON) as
  on x86-64 (AVX2).

## Image tags and variants

The image is `eveyproject/evey`.

| Tag | Published by | Notes |
|---|---|---|
| `X.Y.Z`, `X.Y`, `X`, `latest` | a release tag `vX.Y.Z` | `latest` is always the most recent stable release |
| a pre-release tag (with a hyphen) | a pre-release tag | publishes only its own tag, never moves `latest` |
| a branch name, `sha-<commit>` | a branch build | amd64 only |

Every tag exists in three variants: the default bundles Python 3 for
plugins, `-nodejs` adds Node.js, `-full` adds PHP, Ruby and Lua (for example
`latest-full`). Use a variant only when a plugin needs it: the installer
refuses a plugin whose declared language is missing from the image, with a
message naming the right variant.

Published images carry the drivers for every supported Intel and AMD
generation. A self-built image can keep one family with `docker build
--build-arg GPU=intel-legacy .` (Intel Gen9 to Gen11), `GPU=intel` (Gen12 and
newer), `GPU=amd` or `GPU=none`. NVIDIA needs no variant: the NVIDIA
Container Toolkit injects the host driver at run time.

## The plugin sandbox on an unusual host

The image contains everything needed to confine plugins, without privileged
mode. The default sandbox mode (`auto`) picks the strongest backend the host
allows: Landlock on any modern kernel. Check the mode in force in the
**Security Dashboard** of **Admin -> Plugins**. A host that shows `basic`
cannot isolate plugin file access: [Plugin sandbox
setup](../plugins/sandbox-deployment.md) says what that means and what to
change. The layers themselves are described in [Security model and plugin
sandbox](../SECURITY.md).

<!-- sources
embedded db, import, major upgrade, embedded meili: docker/entrypoint.sh (PG_MAJOR, PG_IMPORT, "evey: import done", EVEY_EMBEDDED_MEILI, MEILI_MASTER_KEY_FILE)
external db switch and default password refusal: internal/config/config.go (EVEY_DB_HOST, hardFail on "evey")
reindex: cmd/evey/admin.go - adminReindexSearch
arm, variants, GPU families: Dockerfile (TARGETARCH, RUNTIMES, GPU) ; .github/workflows/ci.yml (tags, flavor suffix, platforms) ; internal/adapters/driven/ffmpeg/hwdetect.go - detectRKMPP
sandbox auto: internal/adapters/driven/pluginrunner/runner_landlock.go - resolveAutoMode ; i18n/en/admin.yaml - plugins.security.title
-->
