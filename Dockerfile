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
RUN buf dep update && buf generate --template buf.gen.local.yaml

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
RUN gcc -shared -fPIC -nostartfiles -o /libevey-sandbox.so sandbox/sandbox.c

# ── Stage 5: Runtime ──
FROM alpine:3.21.3

RUN apk add --no-cache bubblewrap ca-certificates curl ffmpeg chromaprint postgresql-client unzip

RUN adduser -D -u 1000 evey \
    && adduser -D -u 1001 -G evey evey-plugin

RUN mkdir -p /data/media /data/transcoding /data/logs /data/plugins /data/keys /srv /cache \
    && chown -R evey:evey /data /cache \
    && chown evey-plugin:evey /data/plugins \
    && chmod 750 /data/plugins

COPY --from=builder /evey /evey
COPY --from=builder /libevey-sandbox.so /app/libevey-sandbox.so
COPY --from=builder /src/i18n /i18n
COPY --from=builder /src/sdk /app/sdk
COPY --from=builder /src/docker/seccomp-evey.json /etc/evey/seccomp.json
COPY --from=frontend /app/apps/web/dist /srv

EXPOSE 9091

HEALTHCHECK --interval=10s --timeout=5s --retries=3 --start-period=10s \
    CMD curl -sf http://localhost:9091/healthz || exit 1

USER evey

ENTRYPOINT ["/evey"]
