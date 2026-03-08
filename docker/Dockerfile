FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /evey ./cmd/evey

FROM alpine:3.19

RUN apk add --no-cache \
    ffmpeg \
    libva \
    libva-intel-driver \
    intel-media-driver \
    mesa-va-gallium \
    ca-certificates \
    curl \
    tzdata

COPY --from=builder /evey /usr/local/bin/evey

EXPOSE 8080 50051

ENTRYPOINT ["evey"]
