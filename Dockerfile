# ── Stage 1: Generate protobuf (swagger JSON for orval + Go code for build) ──
FROM golang:1.26.1-alpine AS protogen

RUN apk add --no-cache gcc musl-dev linux-headers
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
RUN go install github.com/bufbuild/buf/cmd/buf@v1.47.2 \
    && go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11 \
    && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1 \
    && go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@v2.28.0 \
    && go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@v2.28.0
COPY proto/ proto/
COPY buf.gen.yaml buf.gen.local.yaml buf.yaml ./
RUN buf generate --template buf.gen.local.yaml

# ── Stage 2: Build frontend ──
FROM node:22-alpine AS frontend

WORKDIR /app
COPY frontend/ .
COPY --from=protogen /src/gen/evey/v1/ /gen/evey/v1/
RUN npm install -g pnpm@10.33.0 && pnpm install \
    && cd packages/api \
    && npx orval --config orval.config.ts \
    && cd ../.. \
    && pnpm --filter web build

# ── Stage 3: Build backend ──
FROM golang:1.26.1-alpine AS builder

RUN apk add --no-cache gcc musl-dev linux-headers

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

ARG VERSION=dev

COPY . .
COPY --from=protogen /src/gen/ gen/
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=${VERSION}" -o /evey ./cmd/evey
RUN gcc -shared -fPIC -nostartfiles -o /libevey-sandbox.so sandbox/sandbox.c

# ── Stage 4: Runtime ──
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
