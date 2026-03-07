.PHONY: proto-gen build test lint docker-build docker-up docker-down clean migrate-up migrate-down

BINARY=evey
DOCKER_COMPOSE=docker compose -f docker/docker-compose.yml

proto-gen:
	buf generate

build:
	go build -o bin/$(BINARY) ./cmd/evey

test:
	go test ./...

lint:
	golangci-lint run ./...

docker-build:
	docker build -f docker/Dockerfile -t evey:latest .

docker-up:
	$(DOCKER_COMPOSE) up -d

docker-down:
	$(DOCKER_COMPOSE) down

clean:
	rm -rf bin/ gen/

migrate-up:
	go run ./cmd/migrate up

migrate-down:
	go run ./cmd/migrate down
