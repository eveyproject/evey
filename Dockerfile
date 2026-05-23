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
RUN apk add --no-cache bubblewrap ca-certificates curl ffmpeg chromaprint postgresql-client unzip wget \
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

RUN mkdir -p /data/media /data/transcoding /data/logs /data/plugins /data/keys /data/meili /srv /cache \
    && chown -R evey:evey /data /cache \
    && chown evey-plugin:evey /data/plugins \
    && chmod 750 /data/plugins

COPY --from=builder /evey /evey
COPY --from=builder /evey-plugin-launcher /evey-plugin-launcher
COPY --from=builder /libevey-sandbox.so /app/libevey-sandbox.so
COPY --from=builder /src/i18n /i18n
COPY --from=builder /src/sdk /app/sdk
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
