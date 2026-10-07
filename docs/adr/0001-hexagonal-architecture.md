# ADR-0001: Hexagonal architecture (ports & adapters)

- Status: Accepted

## Context

Evey targets a long-lived self-hosted product that must evolve while staying testable. We need to swap IO implementations (Postgres, Meilisearch, ffmpeg, TMDB, OMDb, mkvgo, SMTP, web push) without rewriting business logic, and we need unit tests that run in milliseconds without spinning up Docker.

## Decision

Adopt hexagonal architecture with strict layering:

- `internal/domain/<context>/` holds pure models and **ports** (Go interfaces). No framework, no `database/sql`, no HTTP.
- `internal/app/<context>/` holds application services. They depend only on domain ports.
- `internal/adapters/driven/` implements outbound ports (postgres, meilisearch, ffmpeg, mkvgo, opensubtitles, filesystem, email, webpush, webhooksender, pluginrunner).
- `internal/adapters/driving/` implements inbound ports (grpcapi + grpc-gateway, http, pluginhost).
- Composition happens in `cmd/*/wire.go`. Nothing else instantiates concrete adapters.

## Consequences

- Every domain port ships with a fake implementation used in unit tests; adapter-level tests cover real infra separately.
- New bounded contexts follow the same template: `domain/<ctx>` + `app/<ctx>` + adapter additions.
- Cross-context collaboration goes through app-service interfaces or the shared event bus, never direct domain imports.
- The cost is boilerplate (interface + fake + wire binding) - accepted for the testability and evolvability gains.

## References

- [DEV.md](../../DEV.md)
- [docs/ARCHITECTURE.md](../ARCHITECTURE.md)
