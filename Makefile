.PHONY: proto-gen build test lint clean \
	migrate-up migrate-down setup dev dev-down dev-logs \
	docker-build docker-up docker-down frontend-typecheck

BINARY=evey
DC=docker compose --env-file .env.dev

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

# ── Dev ────────────────────────────────────────────────────────────────────────

setup:
	@echo "→ Adding evey.test to WSL /etc/hosts..."
	@grep -q "evey.test" /etc/hosts || echo "127.0.0.1 evey.test" | sudo tee -a /etc/hosts

	@echo "→ Adding evey.test to Windows hosts..."
	@powershell.exe -Command "if (-not (Select-String -Path 'C:\Windows\System32\drivers\etc\hosts' -Pattern 'evey.test' -Quiet)) { Add-Content -Path 'C:\Windows\System32\drivers\etc\hosts' -Value '127.0.0.1 evey.test' }" 2>/dev/null \
		&& echo "  ✓ Windows hosts updated" \
		|| echo "  ⚠ Lance en admin dans PowerShell Windows: Add-Content C:\Windows\System32\drivers\etc\hosts '127.0.0.1 evey.test'"

	@echo ""
	@echo "✓ Setup done → lance 'make dev'"

dev:
	$(DC) down --remove-orphans 2>/dev/null; \
	$(DC) up --build -d

dev-down:
	$(DC) down

dev-logs:
	$(DC) logs -f evey

# ── Infra only (run backend/frontend locally) ─────────────────────────────────

infra:
	$(DC) up postgres meilisearch

# ── Prod ───────────────────────────────────────────────────────────────────────

docker-build:
	docker build -t evey:latest .

docker-up:
	$(DC) --profile prod up -d

docker-down:
	$(DC) --profile prod down

# ── Frontend ───────────────────────────────────────────────────────────────────

frontend-typecheck:
	$(DC) run --rm web pnpm --filter web typecheck
