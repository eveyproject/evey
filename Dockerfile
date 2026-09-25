# ── Stage 1: Go modules (shared cache for protogen + builder) ──
# --platform=$BUILDPLATFORM: everything derived from this stage (generated
# protos, the CGO-free Go binaries) is either architecture-independent or
# cross-compiled from it, so it must run NATIVE on the builder even when the
# image targets another architecture. Without the pin, buildx runs these stages
# under QEMU on a multi-arch build. Measured on one host, same cold build cache,
# same 4 CPUs: `go build ./cmd/evey` for arm64 takes 239 s emulated vs 16 s
# cross-compiled - 15x, and that is one stage out of four.
FROM --platform=$BUILDPLATFORM golang:1.27.0-alpine AS gomod

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

# ── Stage 2b: Build the ac3go WebAssembly decoder (browser AC-3/E-AC-3 path) ──
# Built from the pinned module (go.mod `tool` directive), not committed. The web
# player loads it from /wasm/ac3go.wasm to decode AC-3/E-AC-3 client-side.
# Standard go, NOT TinyGo: TinyGo never runs finalizers, so its js bridge
# leaks every crossing value (+6.75 MB/segment measured 2026-07-28) - that
# memory-killed the cast receiver at ~3 min. Ship wasm_exec.js from the same
# compiler as the module.
FROM --platform=$BUILDPLATFORM golang:1.27.0-alpine AS wasmbuild
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download github.com/gravity-zero/ac3go
RUN mkdir -p /tmp/out \
    && CGO_ENABLED=0 GOOS=js GOARCH=wasm go build -o /tmp/out/ac3go.wasm github.com/gravity-zero/ac3go/cmd/ac3go-wasm \
    && cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" /tmp/out/wasm_exec.js

# ── Stage 3: Build frontend ──
# Output is static JS/CSS/HTML - identical on every target arch, so build it
# natively (see the gomod stage note on $BUILDPLATFORM).
FROM --platform=$BUILDPLATFORM node:22-alpine AS frontend

WORKDIR /app
ENV CI=true
COPY frontend/package.json frontend/pnpm-lock.yaml frontend/.npmrc frontend/pnpm-workspace.yaml ./
COPY frontend/apps/web/package.json apps/web/package.json
COPY frontend/apps/mobile/package.json apps/mobile/package.json
COPY frontend/apps/tv/package.json apps/tv/package.json
COPY frontend/apps/tv-web/package.json apps/tv-web/package.json
COPY frontend/packages/api/package.json packages/api/package.json
COPY frontend/packages/client/package.json packages/client/package.json
COPY frontend/packages/store/package.json packages/store/package.json
COPY frontend/packages/theme/package.json packages/theme/package.json
COPY frontend/patches/ patches/
RUN npm install -g pnpm@10.33.0 && pnpm install --frozen-lockfile
COPY frontend/ .
COPY --from=protogen /src/gen/evey/v1/ /gen/evey/v1/
# The ac3go wasm decoder + Go runtime shim are built (not committed); place them
# in the web app's public dir so vite build ships them at /wasm/.
COPY --from=wasmbuild /tmp/out/ac3go.wasm /tmp/out/wasm_exec.js apps/web/public/wasm/
RUN cd packages/api \
    && npx orval --config orval.config.ts \
    && cd ../.. \
    && pnpm --filter web build \
    && pnpm --filter tv-web build

# ── Stage 4: Build backend ──
# Inherits gomod's $BUILDPLATFORM pin: the Go toolchain runs NATIVE and
# cross-compiles to $TARGETARCH. Legitimate because the whole tree is
# CGO_ENABLED=0 with no `import "C"` anywhere: with cgo off, the Go toolchain
# needs nothing from the host arch, so the cross-compiled binary is the same
# artefact an emulated native build would produce - only far faster.
FROM gomod AS builder

ARG VERSION=dev
# COMMIT / BUILD_DATE stamp the binary with the exact code it carries. The
# defaults are the literal "unknown", never a guess: a build that cannot say
# what it is must SAY so, because a server that reports a plausible-but-wrong
# commit is worse than one that admits ignorance.
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
# Set automatically by buildx; used to cross-compile instead of emulating.
ARG TARGETOS
ARG TARGETARCH

# The lie guard. `--build-arg COMMIT=$(git rev-parse HEAD)` run OUTSIDE a git
# checkout expands to `--build-arg COMMIT=`, which sets COMMIT to the EMPTY
# string and silently overrides the default above: the build then succeeds and
# the image reports "" as its commit - an image that lies. Reject anything that
# is neither a hex git SHA nor the explicit "unknown", so that mistake fails
# here, loudly, instead of six weeks later in front of a production incident.
RUN set -eu; \
    case "${COMMIT}" in \
      unknown) \
        echo "WARNING: building without a commit stamp (COMMIT=unknown) - this image will not be able to identify itself" >&2 ;; \
      *[!0-9a-f]*|"") \
        echo "ERROR: COMMIT build-arg must be a lowercase hex git SHA or the literal 'unknown', got '${COMMIT}'" >&2; exit 1 ;; \
      ???????*) : ;; \
      *) echo "ERROR: COMMIT build-arg '${COMMIT}' is shorter than 7 hex chars" >&2; exit 1 ;; \
    esac; \
    case "${BUILD_DATE}" in \
      unknown) : ;; \
      ????-??-??T??:??:??*) : ;; \
      *) echo "ERROR: BUILD_DATE build-arg must be RFC3339 (e.g. 2026-08-22T10:00:00Z) or the literal 'unknown', got '${BUILD_DATE}'" >&2; exit 1 ;; \
    esac

COPY . .
COPY --from=protogen /src/gen/ gen/
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
      -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildDate=${BUILD_DATE}" \
      -o /evey ./cmd/evey
# Landlock pre-exec shim used by sandbox_mode=landlock (and auto). Pure Go,
# CGO-free; lives next to /evey so the runner finds it via os.Executable().
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o /evey-plugin-launcher ./cmd/evey-plugin-launcher
# mkvgo CLI - the pinned library also ships a command; build it so admins can
# inspect/repair media from inside the container (`mkvgo <TAB>` for commands).
# Version stamped from the go.mod pin so `mkvgo --version` matches the library.
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
      -ldflags="-s -w -X main.version=$(go list -m -f '{{.Version}}' github.com/gravity-zero/mkvgo)" \
      -o /mkvgo github.com/gravity-zero/mkvgo/cmd/mkvgo

# ── Stage 4b: Sandbox shim (glibc) ──
# The Landlock LD_PRELOAD shim is C and links against the runtime's libc. The
# builder above is musl (golang:alpine); a musl .so won't load inside the glibc
# Debian runtime, so compile it here on the same glibc the runtime ships. Kept
# in its own thin stage so gcc + libc6-dev (~200 MB) never reach the final image.
FROM debian:trixie-slim AS sandbox-glibc
RUN apt-get update && apt-get install -y --no-install-recommends gcc libc6-dev \
    && rm -rf /var/lib/apt/lists/*
COPY sandbox/sandbox.c /tmp/sandbox.c
RUN gcc -shared -fPIC -nostartfiles -o /libevey-sandbox.so /tmp/sandbox.c

# ── Stage 4c: Download Meilisearch (glibc release binary) ──
# getmeili/meilisearch is an Alpine image: its /bin/meilisearch is linked
# against musl (interpreter /lib/ld-musl-*.so.1). Copying that binary straight
# into the glibc Debian runtime below produces a file the GNU/Linux dynamic
# linker refuses to load - sh reports "not found" even though the file exists,
# because the *interpreter* is missing, not the binary (reproduced: `docker run
# --rm -v <that binary>:/usr/local/bin/meilisearch debian:trixie-slim
# /usr/local/bin/meilisearch --version` -> exit 127 "not found"). Fetch the
# official GNU/Linux release binary instead (MIT-licensed, published by
# meilisearch/meilisearch on GitHub), pinned to the exact patch version the
# getmeili/meilisearch:v1.12 tag currently resolves to (1.12.8, verified via
# `docker run getmeili/meilisearch:v1.12 meilisearch --version`) so existing
# /data/meili index volumes keep upgrading cleanly. Meilisearch does not
# publish checksums for these release assets, so the pin below is a checksum
# WE computed from that exact downloaded asset - it guards against a
# corrupted download or a same-tag substitution, not against a compromise of
# the release itself.
FROM debian:trixie-slim AS meili-download

ARG TARGETARCH
ARG MEILI_VERSION=v1.12.8

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*

RUN case "$TARGETARCH" in \
      amd64) meili_arch=amd64;   meili_sha256=c325630ea967462e397899bb733a9fb599b89dcc112f8c5cb44d73ac0f16c553 ;; \
      arm64) meili_arch=aarch64; meili_sha256=bea0346b76d354508e8f471b835e472452b0fa8be667fa78727b9b8e4318cc59 ;; \
      *) echo "meili-download: unsupported TARGETARCH '$TARGETARCH'" >&2; exit 1 ;; \
    esac \
    && curl -fsSL -o /usr/local/bin/meilisearch \
         "https://github.com/meilisearch/meilisearch/releases/download/${MEILI_VERSION}/meilisearch-linux-${meili_arch}" \
    && echo "${meili_sha256}  /usr/local/bin/meilisearch" | sha256sum -c - \
    && chmod +x /usr/local/bin/meilisearch \
    # Build-time guard: a musl or wrong-arch binary fails HERE with a
    # non-zero exit (missing interpreter), failing the build immediately
    # instead of surfacing at first container start as it did before - this
    # single line is what would have caught the bug this stage fixes.
    && /usr/local/bin/meilisearch --version

# ── Stage 5: Runtime ──
# ONE image: a single glibc base whose distro ffmpeg has
# every HW encoder compiled in (h264/hevc _nvenc, _vaapi, _qsv - all verified in
# Debian trixie's ffmpeg 7.1). No GPU driver is baked; drivers are injected at
# RUNTIME by whatever the host exposes, so the same image is software-only,
# NVENC, or VAAPI/QSV depending purely on what compose/runtime maps:
#   - NVENC: NVIDIA Container Toolkit (`--gpus all` / a compose nvidia device
#     reservation) + NVIDIA_DRIVER_CAPABILITIES including `video` (set to `all`
#     below) - without `video` the toolkit injects nvidia-smi/CUDA but NOT
#     libnvidia-encode.so and nvenc fails with "Cannot load libnvidia-encode".
#   - VAAPI / QSV: a mapped /dev/dri (host Mesa/iHD); vainfo + the VA/oneVPL
#     drivers are installed below.
# The CGO-free evey + launcher binaries (stage 4, CGO_ENABLED=0) run UNCHANGED on
# glibc; only ffmpeg + the Landlock shim are glibc-specific (the shim is built in
# stage 4b). Debian trixie ships ffmpeg 7.x (scale_cuda's `format=` for the
# 10-bit→nv12 NVENC path) AND postgresql-client-17 (dumps the PG16 server fine -
# newer client, older server) straight from the distro, no PGDG repo dance.
#
# Three variants share this stage; the RUNTIMES build arg picks which plugin
# runtimes the image ships (HW transcode is identical across all three):
#   - lean   (default) : python3 only - covers the majority of community plugins
#   - nodejs           : + node       - for JS plugins (theme frontends, scrapers)
#   - full             : + node + php + ruby + lua - kitchen-sink variant
# Build with:  docker build --build-arg RUNTIMES=full .
# The plugin installer refuses any plugin whose declared `language:` is not
# present, with a message naming the right variant.
FROM debian:trixie-slim AS runtime

ARG RUNTIMES=lean
# Set automatically by buildx per target platform ("amd64" | "arm64"). Used to
# skip the x86-only Intel GPU driver packages and the AVX2 tonemapx assertion
# on arm64, where neither exists - the multi-arch (stable-release) build would
# otherwise fail at those steps. amd64 behaviour is byte-identical.
ARG TARGETARCH
ARG GPU=all

# Tell the NVIDIA Container Toolkit to inject the full driver stack (incl.
# libnvidia-encode for NVENC) when a GPU is reserved; harmless when none is.
ENV NVIDIA_VISIBLE_DEVICES=all
ENV NVIDIA_DRIVER_CAPABILITIES=all

# intel-media-va-driver-non-free (full iHD codec coverage, incl. HEVC/VC1) lives
# in the `non-free` component - enable it on the deb822 sources before apt.
RUN sed -i 's/Components: main/Components: main contrib non-free non-free-firmware/' \
        /etc/apt/sources.list.d/debian.sources

# Base runtime deps + the full VA/oneVPL driver set so VAAPI and QuickSync work
# on whatever GPU the host maps at /dev/dri, with no per-machine rebuild:
#   - mesa-va-drivers               : AMD (radeonsi) + open Intel
#   - intel-media-va-driver-non-free: iHD - Intel Gen8+ (Broadwell and newer)
#   - i965-va-driver                : Intel Gen <= 7.5
#   - libvpl2 + libmfx-gen1.2       : the oneVPL runtime ffmpeg's *_qsv encoders
#                                     dispatch through (trixie dropped the legacy
#                                     libmfx1/MediaSDK - oneVPL supersedes it)
# libva auto-selects the right driver from the GPU's PCI id at runtime -
# LIBVA_DRIVER_NAME is intentionally NOT forced. NVENC needs none of these (CUDA
# via the toolkit); software needs none either.
#
# python3 is bundled in every variant. Plugin signatures (cosign-compatible,
# key-based) are verified in-process by the server - no cosign binary.
#
# GPU picks the accelerator families this image ships (default all):
#   all          every family below
#   intel        Intel VAAPI/QSV + OpenCL tone-map for Gen12 and newer
#   intel-legacy Intel VAAPI/QSV + OpenCL tone-map for Gen9-11 (UHD 6xx)
#   amd          Mesa VAAPI (radeonsi) + Vulkan/RADV for the libplacebo tone-map
#   none         software only
#
# postgresql-17 is the EMBEDDED database (entrypoint: EVEY_EMBEDDED_POSTGRES),
# the same major as the client already shipped for backups. Debian's package
# would create and start a "main" cluster under /var/lib/postgresql at install
# time; postgresql-common's createcluster.conf is flipped first so nothing is
# created - the entrypoint runs initdb itself into /data/postgres, as the
# unprivileged evey user, on first boot. Only the server binaries are wanted
# here (~50 MB): the image stays usable with an external database exactly as
# before, the operator just sets EVEY_DB_HOST.
RUN apt-get update && apt-get -y upgrade && apt-get install -y --no-install-recommends postgresql-common \
    && sed -i 's/^#\?create_main_cluster = .*/create_main_cluster = false/' \
         /etc/postgresql-common/createcluster.conf \
    && grep -q '^create_main_cluster = false' /etc/postgresql-common/createcluster.conf \
    && apt-get install -y --no-install-recommends \
       ca-certificates curl bubblewrap libcap2-bin libchromaprint-tools \
       postgresql-client-17 postgresql-17 unzip wget python3 \
       vainfo libva2 libva-drm2 \
    && case "$GPU" in all|amd) apt-get install -y --no-install-recommends mesa-va-drivers ;; esac \
    # Vulkan for the VAAPI -> Vulkan/libplacebo tone-map route (the Mesa/RADV
    # path of an AMD box, which has no OpenCL ICD): the loader jellyfin-ffmpeg
    # dlopens (libvulkan.so.1) and Mesa's ICDs (radeon_icd = RADV, plus ANV).
    # Nothing here declares the route usable - the calibration derives a Vulkan
    # device from the render node, runs libplacebo on it and measures the
    # picture; a host where that fails simply keeps OpenCL or the CPU bridge.
    && case "$GPU" in all|amd) apt-get install -y --no-install-recommends libvulkan1 mesa-vulkan-drivers \
         && ls /usr/share/vulkan/icd.d/*.json >/dev/null ;; esac \
    # Intel VAAPI/QSV drivers (iHD/i965 + oneVPL) are x86-only - no arm64 package
    # exists in the archive, so install them only on amd64. On arm64 the host's
    # own VAAPI (mesa: a mapped Mali/Rockchip GPU) is used if present; Synology-
    # class ARM has no usable HW encoder, so it falls back to software transcode.
    && if [ "$TARGETARCH" = "amd64" ]; then \
         case "$GPU" in all|intel|intel-legacy) apt-get install -y --no-install-recommends \
           intel-media-va-driver-non-free i965-va-driver libvpl2 libmfx-gen1.2 ;; esac; \
       fi \
    && if [ "$RUNTIMES" = "nodejs" ] || [ "$RUNTIMES" = "full" ]; then \
         apt-get install -y --no-install-recommends nodejs \
         && ([ -e /usr/bin/node ] || ln -sf /usr/bin/nodejs /usr/bin/node); \
       fi \
    && if [ "$RUNTIMES" = "full" ]; then \
         apt-get install -y --no-install-recommends php-cli ruby lua5.4 \
         && ([ -e /usr/bin/lua ] || ln -sf /usr/bin/lua5.4 /usr/bin/lua); \
       fi \
    && rm -rf /var/lib/apt/lists/* \
    # No cluster may exist in the image: the data lives in the /data volume.
    && ! ls /var/lib/postgresql/17/main >/dev/null 2>&1 \
    && /usr/lib/postgresql/17/bin/postgres --version \
    && /usr/lib/postgresql/17/bin/initdb --version >/dev/null

# Embedded Meilisearch - the glibc release binary fetched and build-time
# smoke-tested in the meili-download stage above (NOT the official Alpine
# image: its musl binary can't execute on this glibc runtime, see that
# stage's comment). Pinned to 1.12.8 so existing /data/meili index volumes
# upgrade cleanly.
COPY --from=meili-download /usr/local/bin/meilisearch /usr/local/bin/meilisearch

# jellyfin-ffmpeg - its `tonemapx` filter tonemaps HDR->SDR in a single AVX2/SIMD
# pass, ~1.8x faster than Debian ffmpeg's zscale float32 chain on the SAME CPU
# (measured on an old box, 1080p, same file/encoder: 1.56x vs 0.89x realtime). That
# gap flips 4K HDR software transcode from sub-realtime (buffering, capped to 480p)
# to playable 1080p where the portable zscale path stalls. Installed from the
# jellyfin-ffmpeg project's OFFICIAL Debian repo (the standalone
# jellyfin-ffmpeg7 package - leaner than vendoring the whole Jellyfin image) and
# is the ONLY ffmpeg CLI in the image. The Debian `ffmpeg` package is deliberately
# NOT installed: it was redundant with jellyfin-ffmpeg (shadowed on PATH) and only
# widened the CVE surface on a binary never invoked. The libav* shared libs still
# ship - libchromaprint-tools (fpcalc, audio fingerprinting) hard-depends on them -
# so that path is unaffected; only the redundant CLI is gone. The Go side probes for
# `tonemapx` at runtime (tonemapxAvailable) and falls back to the zscale chain when
# absent, so a build without this layer still works. The build-time smoke test
# fails the image early if the binary can't run or lacks the filter, never in prod.
RUN apt-get update && apt-get install -y --no-install-recommends gnupg \
    && install -d /etc/apt/keyrings \
    && curl -fsSL https://repo.jellyfin.org/jellyfin_team.gpg.key \
         | gpg --dearmor -o /etc/apt/keyrings/jellyfin.gpg \
    && printf 'Types: deb\nURIs: https://repo.jellyfin.org/debian\nSuites: trixie\nComponents: main\nSigned-By: /etc/apt/keyrings/jellyfin.gpg\n' \
         > /etc/apt/sources.list.d/jellyfin.sources \
    && apt-get update && apt-get install -y --no-install-recommends jellyfin-ffmpeg7 \
    && apt-get purge -y gnupg && apt-get autoremove -y && rm -rf /var/lib/apt/lists/* \
    && ln -sf /usr/lib/jellyfin-ffmpeg/ffmpeg  /usr/local/bin/ffmpeg \
    && ln -sf /usr/lib/jellyfin-ffmpeg/ffprobe /usr/local/bin/ffprobe \
    && /usr/local/bin/ffmpeg -hide_banner -version >/dev/null \
    # tonemapx ships in BOTH the amd64 (AVX2) and arm64 (NEON) jellyfin-ffmpeg
    # builds (verified: `.S. tonemapx` is present on aarch64), so assert it on
    # every arch - a build missing it is a broken ffmpeg layer we want to catch.
    # The Go side still probes tonemapxAvailable at runtime and falls back to
    # the zscale chain if a future build ever drops it.
    && /usr/local/bin/ffmpeg -hide_banner -filters | grep -q tonemapx \
    # The Vulkan/libplacebo tone-map route (GPU=all|amd, loader + Mesa ICDs
    # installed above) needs the filter and the hwaccel in THIS binary. Checked
    # without a GPU: presence in the build, never a claim that a device works -
    # the calibration proves that on the host.
    && case "$GPU" in all|amd) /usr/local/bin/ffmpeg -hide_banner -filters | grep -q libplacebo \
         && /usr/local/bin/ffmpeg -hide_banner -hwaccels | grep -q vulkan \
         && [ -e /usr/lib/*/libvulkan.so.1 ] ;; esac

# Intel OpenCL runtime (amd64 only) for tonemap_opencl: the same bt2390 curve as
# the software tone-mapper, run on the GPU, HLG and Dolby Vision RPU included.
# Same pins as Jellyfin's image: NEO main for Gen12+, legacy1 for Gen9-11.
ARG INTEL_GMMLIB_VER=22.10.0
ARG INTEL_IGC2_VER=2.40.13
ARG INTEL_IGC2_BUILD=22418
ARG INTEL_NEO_VER=26.31.39395.13
ARG INTEL_IGC1_LEGACY_VER=1.0.17537.24
ARG INTEL_NEO_LEGACY_VER=24.35.30872.36
RUN set -eu; rm -rf /usr/share/doc /usr/share/man /usr/share/info; \
    if [ "$TARGETARCH" = "amd64" ] && { [ "$GPU" = all ] || [ "$GPU" = intel ] || [ "$GPU" = intel-legacy ]; }; then \
      mkdir /tmp/neo && cd /tmp/neo \
      && curl -fsSLO https://github.com/intel/compute-runtime/releases/download/${INTEL_NEO_VER}/libigdgmm12_${INTEL_GMMLIB_VER}_amd64.deb \
      && if [ "$GPU" != intel-legacy ]; then \
           curl -fsSLO https://github.com/intel/intel-graphics-compiler/releases/download/v${INTEL_IGC2_VER}/intel-igc-core-2_${INTEL_IGC2_VER}+${INTEL_IGC2_BUILD}_amd64.deb \
           && curl -fsSLO https://github.com/intel/intel-graphics-compiler/releases/download/v${INTEL_IGC2_VER}/intel-igc-opencl-2_${INTEL_IGC2_VER}+${INTEL_IGC2_BUILD}_amd64.deb \
           && curl -fsSLO https://github.com/intel/compute-runtime/releases/download/${INTEL_NEO_VER}/intel-opencl-icd_${INTEL_NEO_VER}-0_amd64.deb; \
         fi \
      && if [ "$GPU" != intel ]; then \
           curl -fsSLO https://github.com/intel/intel-graphics-compiler/releases/download/igc-${INTEL_IGC1_LEGACY_VER}/intel-igc-core_${INTEL_IGC1_LEGACY_VER}_amd64.deb \
           && curl -fsSLO https://github.com/intel/intel-graphics-compiler/releases/download/igc-${INTEL_IGC1_LEGACY_VER}/intel-igc-opencl_${INTEL_IGC1_LEGACY_VER}_amd64.deb \
           && curl -fsSLO https://github.com/intel/compute-runtime/releases/download/${INTEL_NEO_LEGACY_VER}/intel-opencl-icd-legacy1_${INTEL_NEO_LEGACY_VER}_amd64.deb; \
         fi \
      && apt-get update && apt-get install -y --no-install-recommends ./*.deb \
      && cd / && rm -rf /tmp/neo /var/lib/apt/lists/* /usr/share/doc /usr/share/man \
      # The legacy IGC ships libopencl-clang.so as a 137 MB COPY of the .so.14, not a link.
      && { [ ! -e /usr/local/lib/libopencl-clang.so.14 ] || ln -sf libopencl-clang.so.14 /usr/local/lib/libopencl-clang.so; } \
      && ls /etc/OpenCL/vendors/ \
      && /usr/local/bin/ffmpeg -hide_banner -filters | grep -q tonemap_opencl; \
    fi

RUN groupadd -g 1000 evey \
    && useradd -u 1000 -g evey -m -s /usr/sbin/nologin evey \
    && useradd -u 1001 -g evey -m -s /usr/sbin/nologin evey-plugin \
    && mkdir -p /data/transcoding /data/logs /data/plugins /data/keys /data/meili /data/postgres /data/backups /srv /cache \
    && chown -R evey:evey /data /cache \
    # The SERVER owns /data/plugins: it extracts uploaded plugins there
    # (evey-plugin ownership denied it mkdir and let a plugin tamper with its
    # own validated files). 750 = group evey (which evey-plugin belongs to)
    # reads/execs the installed files, others get nothing.
    && chmod 750 /data/plugins \
    # Owner-only (no group/other): with the plugin dropped to uid 1001, plain
    # DAC keeps these secret-bearing dirs unreadable to plugins even without
    # Landlock. jwt.key is already 0600; this covers backups (full DB) + logs.
    && chmod 700 /data/keys /data/logs /data/backups /data/postgres

# Secrets the image generates on first boot (entrypoint) live under /data/keys;
# the server reads them through these *_FILE variables when the plain variable
# is unset (config.go envSecret). Baked into the image, not exported by the
# entrypoint, so `docker exec evey-server evey admin ...` finds the same
# database password the server uses. An explicit EVEY_DB_PASSWORD /
# MEILI_MASTER_KEY in compose always takes precedence over the file.
ENV EVEY_DB_PASSWORD_FILE=/data/keys/postgres.password \
    MEILI_MASTER_KEY_FILE=/data/keys/meili.key

# Container paths for everything the server writes, baked so a minimal compose
# (volumes only, no environment) lands every artefact on the right volume. The
# Go defaults are built for a bare-metal run and are wrong in the container:
# EVEY_LOG_DIR defaults to the RELATIVE "logs" (cwd is /, unwritable once
# privileges drop - file logging then fails with "mkdir logs: permission
# denied" and the admin log viewer stays empty, which is exactly how this was
# found on a prod install), and the two cache dirs default to each other's
# volume (transcode cache into the precious /data, trickplay into the prunable
# /cache). Transcode cache is expendable (/cache); trickplay is expensive to
# regenerate and stays with /data. A compose `environment:` entry still
# overrides any of these.
ENV EVEY_LOG_DIR=/data/logs \
    EVEY_TRANSCODING_CACHE_DIR=/cache/transcode \
    EVEY_TRICKPLAY_CACHE_DIR=/data/trickplay

COPY --from=builder /evey /evey
COPY --from=builder /evey-plugin-launcher /evey-plugin-launcher
# cap_setuid lets the launcher drop a plugin to the evey-plugin uid (1001) even
# when the server runs non-root, enabling EVEY_PLUGINS_UID_SEPARATION=true so a
# plugin can't read the server's 0600 secrets on a host without Landlock.
# Dormant unless that flag is set; the launcher refuses any target uid < 1000.
RUN setcap cap_setuid+ep /evey-plugin-launcher
COPY --from=sandbox-glibc /libevey-sandbox.so /app/libevey-sandbox.so
COPY --from=builder /src/i18n /i18n
COPY --from=builder /src/sdk /app/sdk
# Generated OpenAPI v2 specs, served (filtered to the API-key surface) at
# /api/v1/openapi.json. Matches publicOpenAPIDirs in openapi_handler.go;
# without this the endpoint 503s (the specs live only in the build stages).
COPY --from=builder /src/gen/evey/v1 /app/gen/evey/v1
COPY --from=builder /src/docker/seccomp-evey.json /etc/evey/seccomp.json
COPY --from=frontend /app/apps/web/dist /srv
# The television client, the same build a webOS/Tizen package wraps: reachable
# from any TV browser at <server>/tv, installable as a PWA where the browser
# allows it.
COPY --from=frontend /app/apps/tv-web/dist /srv/tv

# Admin shell polish shown on `docker exec -it evey-server bash`: tab-completion
# for the `evey` maintenance CLI and the `mkvgo` media tool, plus a one-time
# greeting (banner + both command groups). The `evey` binary is already at /evey;
# symlink it onto PATH so `evey admin …` works from any cwd. All sourced from the
# system bashrc - nothing here touches the running server.
RUN ln -sf /evey /usr/local/bin/evey
COPY --from=builder /mkvgo /usr/local/bin/mkvgo
COPY docker/evey-completion.bash /etc/bash_completion.d/evey
COPY docker/mkvgo-completion.bash /etc/bash_completion.d/mkvgo
COPY docker/evey-shell.sh /etc/evey/shell.sh
RUN printf '\n# Evey admin shell: CLI completion + interactive launcher\nfor _f in /etc/bash_completion.d/evey /etc/bash_completion.d/mkvgo; do [ -r "$_f" ] && . "$_f"; done\n[ -r /etc/evey/shell.sh ] && . /etc/evey/shell.sh\n' >> /etc/bash.bashrc

COPY docker/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Surface the variant for `docker inspect` debugging without forcing a rebuild
# to find out which one is running. Lowercased so labels stay grep-friendly.
LABEL org.evey.runtimes="${RUNTIMES}" org.evey.gpu="${GPU}"
ENV EVEY_IMAGE_RUNTIMES=${RUNTIMES}

# Same build stamps the binary carries, on the image itself: standard OCI keys
# so `docker inspect` / registry UIs answer "which commit is this tag?" without
# starting a container. ARGs are per-stage, hence the re-declaration.
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.source="https://github.com/evey-project/evey"
# The repository this image is pushed to: the update check reads what the
# registry publishes under the running tag there.
ARG IMAGE_REPO=docker.io/gravityz/evey
ENV EVEY_IMAGE_REPO=${IMAGE_REPO}

EXPOSE 9091

# start-period covers the SLOWEST legitimate boot, not the typical one: a
# release carrying a schema migration first writes a full pre-migration
# pg_dump of the catalogue (keyframe arrays included), and on a NAS-sized
# library that alone can outlast a 30s window - the server was then declared
# unhealthy while it was correctly working, and `docker compose up` aborted
# the dependency chain. Failures inside the window don't count, so a generous
# value costs nothing on a fast boot.
HEALTHCHECK --interval=10s --timeout=5s --retries=3 --start-period=300s \
    CMD curl -sf http://localhost:9091/healthz || exit 1

# No static USER: the entrypoint starts as root just long enough to re-map
# the evey user to the operator's PUID/PGID (defaults 1000:1000), then
# re-execs itself unprivileged via setpriv - the server process NEVER runs
# as root. Operators who refuse the transient root can set `user:` in
# compose; the entrypoint detects the non-root uid and skips the re-map.
ENTRYPOINT ["/entrypoint.sh"]
