FROM golang:1.25.8-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

RUN go install github.com/bufbuild/buf/cmd/buf@v1.47.2

COPY . .
RUN buf generate
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /evey ./cmd/evey
RUN gcc -shared -fPIC -nostartfiles -o /libevey-sandbox.so sandbox/sandbox.c

FROM alpine:3.21.3

RUN apk add --no-cache ffmpeg ca-certificates unzip curl postgresql-client

RUN adduser -D -u 1000 evey

RUN mkdir -p /data/media /data/transcoding /data/logs /data/plugins /data/keys \
    && chown -R evey:evey /data

COPY --from=builder /evey /evey
COPY --from=builder /libevey-sandbox.so /app/libevey-sandbox.so

EXPOSE 9090 9091

HEALTHCHECK --interval=10s --timeout=5s --retries=3 --start-period=10s \
    CMD curl -sf http://localhost:9091/healthz || exit 1

USER evey

ENTRYPOINT ["/evey"]
