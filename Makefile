.PHONY: proto-gen build test lint docker-build docker-up docker-down clean \
	migrate-up migrate-down setup dev dev-down frontend-typecheck

BINARY=evey
DOCKER_COMPOSE=docker compose -f docker/docker-compose.yml
DOCKER_COMPOSE_DEV=docker compose -f docker/docker-compose.dev.yml

# ── Build ──────────────────────────────────────────────────────────────────────

proto-gen:
	buf generate

build:
	go build -o bin/$(BINARY) ./cmd/evey

test:
	go test ./...

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/ gen/

# ── Database ───────────────────────────────────────────────────────────────────

migrate-up:
	go run ./cmd/migrate up

migrate-down:
	go run ./cmd/migrate down

# ── Production Docker ──────────────────────────────────────────────────────────

docker-build:
	docker build -f docker/Dockerfile -t evey:latest .

docker-up:
	$(DOCKER_COMPOSE) up -d

docker-down:
	$(DOCKER_COMPOSE) down

# ── Dev setup (à lancer une fois) ─────────────────────────────────────────────

setup:
	@echo "→ Adding evey.test to WSL /etc/hosts..."
	@grep -q "evey.test" /etc/hosts || echo "127.0.0.1 evey.test" | sudo tee -a /etc/hosts

	@echo "→ Adding evey.test to Windows hosts..."
	@powershell.exe -Command "if (-not (Select-String -Path 'C:\Windows\System32\drivers\etc\hosts' -Pattern 'evey.test' -Quiet)) { Add-Content -Path 'C:\Windows\System32\drivers\etc\hosts' -Value '127.0.0.1 evey.test' }" 2>/dev/null \
		&& echo "  ✓ Windows hosts updated" \
		|| echo "  ⚠ Lance en admin dans PowerShell Windows: Add-Content C:\Windows\System32\drivers\etc\hosts '127.0.0.1 evey.test'"

	@echo ""
	@echo "✓ Setup done → lance 'make dev'"

# ── Dev ───────────────────────────────────────────────────────────────────────

dev:
	@EXTRA=""; [ -f docker/docker-compose.media.yml ] && EXTRA="-f docker/docker-compose.media.yml"; \
	$(DOCKER_COMPOSE_DEV) $$EXTRA down --remove-orphans 2>/dev/null; \
	$(DOCKER_COMPOSE_DEV) $$EXTRA up --build

dev-down:
	$(DOCKER_COMPOSE_DEV) down

# ── Frontend ───────────────────────────────────────────────────────────────────

frontend-typecheck:
	$(DOCKER_COMPOSE_DEV) run --rm web pnpm --filter web typecheck
