# ── Stage 1: Build frontend ──
FROM node:22-alpine AS frontend

WORKDIR /app
COPY frontend/ .
RUN npm install -g pnpm@10.6.5 && pnpm install && pnpm --filter web build

# ── Stage 2: Build backend ──
FROM golang:1.26.1-alpine AS builder

RUN apk add --no-cache gcc musl-dev linux-headers

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

RUN go install github.com/bufbuild/buf/cmd/buf@v1.47.2

ARG VERSION=dev

COPY . .
RUN buf generate
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=${VERSION}" -o /evey ./cmd/evey
RUN gcc -shared -fPIC -nostartfiles -o /libevey-sandbox.so sandbox/sandbox.c

# ── Stage 3: Runtime ──
FROM alpine:3.21.3

RUN apk add --no-cache ffmpeg ca-certificates unzip curl postgresql-client

RUN adduser -D -u 1000 evey \
    && adduser -D -u 1001 -G evey evey-plugin

RUN mkdir -p /data/media /data/transcoding /data/logs /data/plugins /data/keys /srv \
    && chown -R evey:evey /data \
    && chown evey-plugin:evey /data/plugins \
    && chmod 750 /data/plugins

COPY --from=builder /evey /evey
COPY --from=builder /libevey-sandbox.so /app/libevey-sandbox.so
COPY --from=frontend /app/apps/web/dist /srv

EXPOSE 9091

HEALTHCHECK --interval=10s --timeout=5s --retries=3 --start-period=10s \
    CMD curl -sf http://localhost:9091/healthz || exit 1

USER evey

ENTRYPOINT ["/evey"]
