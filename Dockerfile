# ── Stage 1: Go modules (shared cache for protogen + builder) ──
FROM golang:1.26.3-alpine AS gomod

RUN apk add --no-cache gcc musl-dev linux-headers
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

# ── Stage 2: Generate protobuf ──
FROM gomod AS protogen

RUN go install github.com/bufbuild/buf/cmd/buf@v1.47.2 \
    && go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11 \
    && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1 \
    && go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@v2.28.0 \
    && go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@v2.28.0
COPY proto/ proto/
COPY buf.gen.yaml buf.gen.local.yaml buf.yaml buf.lock ./
RUN buf generate --template buf.gen.local.yaml

# ── Stage 3: Build frontend ──
FROM node:22-alpine AS frontend

WORKDIR /app
ENV CI=true
COPY frontend/package.json frontend/pnpm-lock.yaml frontend/.npmrc frontend/pnpm-workspace.yaml ./
COPY frontend/apps/web/package.json apps/web/package.json
COPY frontend/apps/mobile/package.json apps/mobile/package.json
COPY frontend/apps/tv/package.json apps/tv/package.json
COPY frontend/packages/api/package.json packages/api/package.json
COPY frontend/packages/store/package.json packages/store/package.json
COPY frontend/packages/theme/package.json packages/theme/package.json
COPY frontend/patches/ patches/
RUN npm install -g pnpm@10.33.0 && pnpm install --frozen-lockfile
COPY frontend/ .
COPY --from=protogen /src/gen/evey/v1/ /gen/evey/v1/
RUN cd packages/api \
    && npx orval --config orval.config.ts \
    && cd ../.. \
    && pnpm --filter web build

# ── Stage 4: Build backend ──
FROM gomod AS builder

ARG VERSION=dev

COPY . .
COPY --from=protogen /src/gen/ gen/
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=${VERSION}" -o /evey ./cmd/evey
# Landlock pre-exec shim used by sandbox_mode=landlock (and auto). Pure Go,
# CGO-free; lives next to /evey so the runner finds it via os.Executable().
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /evey-plugin-launcher ./cmd/evey-plugin-launcher
RUN gcc -shared -fPIC -nostartfiles -o /libevey-sandbox.so sandbox/sandbox.c

# ── Stage 5: Runtime ──
# Three variants share this stage; the RUNTIMES build arg picks which plugin
# runtimes the image ships:
#   - lean   (default) : python3 only — covers the majority of community plugins
#   - nodejs           : + nodejs    — for JS plugins (theme frontends, scrapers)
#   - full             : + nodejs + php + ruby + lua — kitchen-sink variant
# Build with:  docker build --build-arg RUNTIMES=full .
# The plugin installer refuses any plugin whose declared `language:` is not
# present, with a message naming the right variant.
FROM alpine:3.21.3

ARG RUNTIMES=lean

# Base runtime deps + the full open VAAPI driver set so hardware transcode
# works on whatever GPU the host has, with no image rebuild per machine:
#   - libva / libva-utils  : VAAPI core + `vainfo` for diagnostics
#   - intel-media-driver   : iHD — Intel Broadwell (Gen8) and newer
#   - libva-intel-driver   : i965 — Intel old iGPU (Gen7.5) and older
#   - mesa-va-gallium      : AMD VAAPI
# libva auto-selects the right driver from the GPU's PCI ID at runtime —
# LIBVA_DRIVER_NAME is intentionally NOT forced. Alpine's ffmpeg ships with
# --enable-vaapi, so `-hwaccel vaapi` covers Intel QuickSync and AMD alike;
# NVENC needs the host NVIDIA driver via nvidia-container-toolkit (separate).
#
# python3 is bundled in every variant: the majority of community plugins use
# it, and the install error message points users to `evey:beta-nodejs` /
# `evey:beta-full` if they need something heavier.
# cosign verifies plugin signatures when EVEY_PLUGINS_COSIGN_POLICY is warn/enforce.
# Without it, enforce fails closed and blocks EVERY install (the verifier shells out
# to `cosign verify-blob`). See tests/plugins/cosign/README.md for the signing flow.
RUN apk add --no-cache bubblewrap libcap cosign ca-certificates curl ffmpeg chromaprint postgresql-client unzip wget \
    libva libva-utils intel-media-driver libva-intel-driver mesa-va-gallium \
    python3 \
    && if [ "$RUNTIMES" = "nodejs" ] || [ "$RUNTIMES" = "full" ]; then \
         apk add --no-cache nodejs npm; \
       fi \
    && if [ "$RUNTIMES" = "full" ]; then \
         apk add --no-cache php83 php83-cli ruby lua5.4; \
       fi

# Embedded Meilisearch — copied from the official image so we ride their
# release cadence + security patching, no extra download stage to maintain.
# Pinned to the same version the previous docker-compose used (v1.12) so
# existing /data/meili index volumes upgrade cleanly across rebuilds.
COPY --from=getmeili/meilisearch:v1.12 /bin/meilisearch /usr/local/bin/meilisearch

RUN adduser -D -u 1000 evey \
    && adduser -D -u 1001 -G evey evey-plugin

RUN mkdir -p /data/transcoding /data/logs /data/plugins /data/keys /data/meili /data/backups /srv /cache \
    && chown -R evey:evey /data /cache \
    && chown evey-plugin:evey /data/plugins \
    && chmod 750 /data/plugins \
    # Owner-only (no group/other): with the plugin dropped to uid 1001, plain
    # DAC keeps these secret-bearing dirs unreadable to plugins even without
    # Landlock. jwt.key is already 0600; this covers backups (full DB) + logs.
    && chmod 700 /data/keys /data/logs /data/backups

COPY --from=builder /evey /evey
COPY --from=builder /evey-plugin-launcher /evey-plugin-launcher
# cap_setuid lets the launcher drop a plugin to the evey-plugin uid (1001) even
# when the server runs non-root, enabling EVEY_PLUGINS_UID_SEPARATION=true so a
# plugin can't read the server's 0600 secrets on a host without Landlock.
# Dormant unless that flag is set; the launcher refuses any target uid < 1000.
# Requires the container to keep CAP_SETUID in its bounding set (Docker default;
# don't cap_drop it) and is incompatible with no-new-privileges.
RUN setcap cap_setuid+ep /evey-plugin-launcher
COPY --from=builder /libevey-sandbox.so /app/libevey-sandbox.so
COPY --from=builder /src/i18n /i18n
COPY --from=builder /src/sdk /app/sdk
# Generated OpenAPI v2 specs, served (filtered to the API-key surface) at
# /api/v1/openapi.json. Matches publicOpenAPIDirs in openapi_handler.go;
# without this the endpoint 503s (the specs live only in the build stages).
COPY --from=builder /src/gen/evey/v1 /app/gen/evey/v1
COPY --from=builder /src/docker/seccomp-evey.json /etc/evey/seccomp.json
COPY --from=frontend /app/apps/web/dist /srv
COPY docker/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Surface the variant for `docker inspect` debugging without forcing a
# rebuild to find out which one is running. Lowercased on purpose so the
# image labels stay grep-friendly.
LABEL org.evey.runtimes="${RUNTIMES}"
ENV EVEY_IMAGE_RUNTIMES=${RUNTIMES}

EXPOSE 9091

HEALTHCHECK --interval=10s --timeout=5s --retries=3 --start-period=30s \
    CMD curl -sf http://localhost:9091/healthz || exit 1

USER evey

ENTRYPOINT ["/entrypoint.sh"]

# ── Stage 6: NVIDIA / glibc runtime (opt-in, hardware NVENC transcode) ──
# The default Alpine runtime cannot do NVENC: musl can't load the glibc NVIDIA
# driver libraries the nvidia-container-toolkit / Docker Desktop injects at
# `--gpus` time, and Alpine's ffmpeg isn't built with --enable-nvenc. This
# glibc base can. The evey + launcher binaries are CGO-free/static (built with
# CGO_ENABLED=0 in stage 4) so they run here UNCHANGED — only ffmpeg and the
# Landlock sandbox shim must be glibc, which this stage rebuilds.
#
# Built + run via docker-compose.override.yml's `evey-hw` service. The base
# Alpine image (stage 5) stays the production default and is untouched.
FROM nvidia/cuda:12.6.2-base-ubuntu24.04 AS runtime-nvidia

# Expose the NVENC/NVDEC engines to the container: the cuda base advertises
# `compute,utility` by default; `video` is the capability ffmpeg's *_nvenc
# encoders need.
ENV NVIDIA_VISIBLE_DEVICES=all
ENV NVIDIA_DRIVER_CAPABILITIES=compute,utility,video

RUN apt-get update && apt-get install -y --no-install-recommends \
    ffmpeg bubblewrap libcap2-bin ca-certificates curl libchromaprint-tools \
    postgresql-client unzip wget python3 gcc libc6-dev \
    && rm -rf /var/lib/apt/lists/*

# Rebuild the Landlock pre-exec shim with the glibc toolchain — the musl
# .so from stage 4 won't load under glibc. Same flags as stage 4.
# libc6-dev (above) pulls linux-libc-dev for the kernel UAPI headers
# (linux/seccomp.h, linux/filter.h) the shim includes.
COPY --from=builder /src/sandbox/sandbox.c /tmp/sandbox.c
RUN mkdir -p /app && gcc -shared -fPIC -nostartfiles -o /app/libevey-sandbox.so /tmp/sandbox.c && rm /tmp/sandbox.c

COPY --from=getmeili/meilisearch:v1.12 /bin/meilisearch /usr/local/bin/meilisearch

# Ubuntu 24.04 base images ship a default `ubuntu` user at UID 1000 — remove it
# so our evey/evey-plugin UIDs (1000/1001) match the Alpine runtime exactly.
RUN (userdel -r ubuntu 2>/dev/null || true) && (groupdel ubuntu 2>/dev/null || true) \
    && groupadd -g 1000 evey \
    && useradd -u 1000 -g evey -m -s /usr/sbin/nologin evey \
    && useradd -u 1001 -g evey -m -s /usr/sbin/nologin evey-plugin \
    && mkdir -p /data/transcoding /data/logs /data/plugins /data/keys /data/meili /data/backups /srv /cache \
    && chown -R evey:evey /data /cache \
    && chown evey-plugin:evey /data/plugins \
    && chmod 750 /data/plugins \
    && chmod 700 /data/keys /data/logs /data/backups

COPY --from=builder /evey /evey
COPY --from=builder /evey-plugin-launcher /evey-plugin-launcher
RUN setcap cap_setuid+ep /evey-plugin-launcher
COPY --from=builder /src/i18n /i18n
COPY --from=builder /src/sdk /app/sdk
COPY --from=builder /src/gen/evey/v1 /app/gen/evey/v1
COPY --from=builder /src/docker/seccomp-evey.json /etc/evey/seccomp.json
COPY --from=frontend /app/apps/web/dist /srv
COPY docker/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

LABEL org.evey.runtimes="nvidia"
ENV EVEY_IMAGE_RUNTIMES=nvidia

EXPOSE 9091

HEALTHCHECK --interval=10s --timeout=5s --retries=3 --start-period=30s \
    CMD curl -sf http://localhost:9091/healthz || exit 1

USER evey

ENTRYPOINT ["/entrypoint.sh"]

# ── Stage 7: VAAPI / glibc runtime (opt-in, AMD iGPU / Intel QSV transcode) ──
# Sibling of runtime-nvidia, for VAAPI hardware. On a real Linux host (a NAS)
# the default Alpine image already does VAAPI via the native amdgpu/i915 + its
# bundled mesa; this glibc variant exists for the WSL2 dev box, where AMD/Intel
# VAAPI is only reachable through Mesa's d3d12 driver — which needs Mesa < 24
# (the host's Mesa 25 hits the #11838 init bug). Ubuntu 22.04 ships Mesa 23.x,
# so the d3d12 VA driver initialises. At runtime the compose service maps
# /dev/dri + /dev/dxg and mounts the host's /usr/lib/wsl (the WSL GPU libs) on
# LD_LIBRARY_PATH, with LIBVA_DRIVER_NAME=d3d12. The CGO-free evey binary and
# the static launcher are reused unchanged; only ffmpeg + the Landlock shim are
# rebuilt against glibc.
FROM ubuntu:22.04 AS runtime-vaapi

# d3d12 is the WSL2 VAAPI backend; native hosts ignore this and auto-select
# radeonsi/iHD/i965 from the GPU PCI id, so it is harmless on a NAS too.
ENV LIBVA_DRIVER_NAME=d3d12

# postgresql-client-16 from PGDG (Ubuntu 22.04 only ships pg_dump 14, which
# refuses to dump the Postgres 16 server — the pre-migration backup would fail).
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl gnupg \
    && install -d /usr/share/postgresql-common/pgdg \
    && curl -fsSL https://www.postgresql.org/media/keys/ACCC4CF8.asc -o /usr/share/postgresql-common/pgdg/apt.postgresql.org.asc \
    && echo "deb [signed-by=/usr/share/postgresql-common/pgdg/apt.postgresql.org.asc] https://apt.postgresql.org/pub/repos/apt jammy-pgdg main" > /etc/apt/sources.list.d/pgdg.list \
    && apt-get update && apt-get install -y --no-install-recommends \
       ffmpeg bubblewrap libcap2-bin libchromaprint-tools \
       postgresql-client-16 unzip wget python3 gcc libc6-dev \
       vainfo mesa-va-drivers libva2 libva-drm2 \
    && rm -rf /var/lib/apt/lists/*

# Rebuild the Landlock shim with glibc (musl .so from stage 4 won't load).
COPY --from=builder /src/sandbox/sandbox.c /tmp/sandbox.c
RUN mkdir -p /app && gcc -shared -fPIC -nostartfiles -o /app/libevey-sandbox.so /tmp/sandbox.c && rm /tmp/sandbox.c

COPY --from=getmeili/meilisearch:v1.12 /bin/meilisearch /usr/local/bin/meilisearch

RUN (userdel -r ubuntu 2>/dev/null || true) && (groupdel ubuntu 2>/dev/null || true) \
    && groupadd -g 1000 evey \
    && useradd -u 1000 -g evey -m -s /usr/sbin/nologin evey \
    && useradd -u 1001 -g evey -m -s /usr/sbin/nologin evey-plugin \
    && mkdir -p /data/transcoding /data/logs /data/plugins /data/keys /data/meili /data/backups /srv /cache \
    && chown -R evey:evey /data /cache \
    && chown evey-plugin:evey /data/plugins \
    && chmod 750 /data/plugins \
    && chmod 700 /data/keys /data/logs /data/backups

COPY --from=builder /evey /evey
COPY --from=builder /evey-plugin-launcher /evey-plugin-launcher
RUN setcap cap_setuid+ep /evey-plugin-launcher
COPY --from=builder /src/i18n /i18n
COPY --from=builder /src/sdk /app/sdk
COPY --from=builder /src/gen/evey/v1 /app/gen/evey/v1
COPY --from=builder /src/docker/seccomp-evey.json /etc/evey/seccomp.json
COPY --from=frontend /app/apps/web/dist /srv
COPY docker/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

LABEL org.evey.runtimes="vaapi"
ENV EVEY_IMAGE_RUNTIMES=vaapi

EXPOSE 9091

HEALTHCHECK --interval=10s --timeout=5s --retries=3 --start-period=30s \
    CMD curl -sf http://localhost:9091/healthz || exit 1

USER evey

ENTRYPOINT ["/entrypoint.sh"]
