FROM golang:1.25-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /evey ./cmd/evey

FROM alpine:3.21

RUN apk add --no-cache ffmpeg ca-certificates unzip curl

RUN adduser -D -u 1000 evey

RUN mkdir -p /data/media /data/transcoding /data/logs /data/plugins /data/keys \
    && chown -R evey:evey /data

COPY --from=builder /evey /evey

EXPOSE 9090 9091

HEALTHCHECK --interval=10s --timeout=5s --retries=3 --start-period=10s \
    CMD curl -sf http://localhost:9091/healthz || exit 1

USER evey

ENTRYPOINT ["/evey"]
