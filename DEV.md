# Evey - Developer Guide

## Prerequisites

- Go 1.26.1+
- Node.js 22+ / pnpm 10+
- Docker & Docker Compose
- [buf](https://buf.build/) (protobuf generation)
- [golangci-lint](https://golangci-lint.run/) (linting)
- [Air](https://github.com/air-verse/air) (hot reload, bundled in dev container)

## Quick Start

```bash
# First time only - create your dev environment file. Every compose-driven make
# target passes --env-file .env.dev, so without it `make dev` aborts before
# Docker starts. The file is gitignored: it holds your own secrets and paths.
cp .env.dev.example .env.dev
# Then edit at least MEDIA_DIR, EVEY_DB_PASSWORD and MEILI_MASTER_KEY.

# First time only - adds evey.test to /etc/hosts
make setup

# Start everything (postgres, meilisearch, backend with Air, frontend with Vite)
make dev

# Open https://evey.test (Caddy) or http://localhost:5173 (Vite direct)
```

## Architecture

Hexagonal (ports & adapters), Go backend with gRPC + grpc-gateway.

```
cmd/evey/              Entry point, wire, HTTP/gRPC servers
internal/domain/<ctx>/ Domain models + port interfaces + fakes
internal/app/<ctx>/    Application services (business logic)
internal/adapters/
  driven/postgres/     Database repositories + migrations
  driven/meilisearch/  Search adapter
  driven/metadata/     TMDB/OMDb clients
  driving/grpcapi/     gRPC handlers
  driving/http/        HTTP gateway, rate limiting, CSRF, auth
frontend/
  apps/web/            React 19 + Vite + TanStack Router/Query + Tailwind v4
  apps/mobile/         Expo 55 + React Native (Android)
  apps/tv/             Expo 55 + React Native TVOS (Android TV)
  packages/api/        Generated API client (orval)
  packages/theme/      Shared CSS variables
  packages/store/      Shared Zustand stores
proto/evey/v1/         Protobuf definitions
gen/evey/v1/           Generated code (DO NOT EDIT)
tests/e2e/             Playwright E2E tests
i18n/                  Backend translation files (YAML, 6 languages)
```

## Ports

| Service | Port |
|---------|------|
| gRPC server | 9090 |
| HTTP gateway / API | 9091 |
| Vite dev server | 5173 |
| PostgreSQL | 5432 |
| Meilisearch | 7700 |
| Caddy (HTTPS) | 443 |

## Make Commands

| Command | Description |
|---------|-------------|
| `make dev` | Start full dev environment (docker compose) |
| `make dev-down` | Stop dev environment |
| `make dev-logs` | Tail backend logs |
| `make infra` | Start only postgres + meilisearch (run backend locally) |
| `make build` | Build backend binary to `bin/evey` |
| `make test` | Run all Go tests with `-race` |
| `make lint` | Run golangci-lint |
| `make proto-gen` | Generate protobuf code with buf |
| `make migrate-up` | Run database migrations |
| `make migrate-down` | Rollback last migration |
| `make frontend-typecheck` | TypeScript check (all frontend apps) |
| `make plugin-sdk` | Generate plugin SDK stubs (Python, Node, PHP, Ruby, Java, Swift, Dart) |

## Testing

### Backend
```bash
make test                                          # All packages
go test ./internal/app/library/... -race -count=1  # Single package
```

### Frontend
```bash
cd frontend && pnpm --filter web test              # Unit tests (1082 tests)
```

### E2E
```bash
cd tests/e2e && npx playwright test                # All specs (2009 tests, 104 specs)
npx playwright test scenarios/media/ --workers=1   # Specific suite
MONKEY_RUNS=100 npx playwright test scenarios/monkey-testing.spec.ts --timeout=45000 --workers=1
```

## Code Conventions

### Go
- Structured logging with `slog` (never `fmt.Println` or `log`)
- Hexagonal: domain depends on nothing, app depends on domain ports, adapters implement ports
- Fakes for testing in `internal/domain/<ctx>/fakes/`
- `checkLibraryAccess()` on every handler accepting `library_id` or `media_id`
- Error wrapping: `fmt.Errorf("context: %w", err)`
- `golangci-lint` must pass with 0 issues on new code

### Frontend
- `eveyFetch` wrapper (never raw `fetch`) - handles token refresh
- Colors via CSS vars (`var(--accent)`) - never hardcoded hex
- `useNavigate` from TanStack Router (never `window.location`)
- `async/await` + `try/catch` (never `.then().catch()`)
- i18n: all user-visible strings via `useTranslation` / `t()`

## Pre-push Checklist

```bash
go vet $(go list ./... | grep -v /gen/)
cd frontend && pnpm run typecheck
go build -o /tmp/evey-check ./cmd/evey
```

## Protobuf Workflow

```bash
# Edit proto files in proto/evey/v1/
# Then regenerate:
make proto-gen
# Generated code lands in gen/evey/v1/ - never edit manually
```

## Docker (Production Build)

```bash
docker build -t evey .
# 7 stages: gomod -> protogen -> wasmbuild -> frontend -> builder
#           -> sandbox-glibc -> runtime
# The runtime is debian:trixie-slim (not alpine): use apt, not apk, when
# debugging inside it. Only the build stages are alpine-based.
# Single image serves both API and frontend SPA
```

## Useful Tools

| Tool | Purpose |
|------|---------|
| `golangci-lint` | Meta-linter |
| `govulncheck` | CVE scanner |
| `deadcode` | Find unused code |
| `gocognit` / `gocyclo` | Complexity metrics |
| `gremlins` | Mutation testing |
