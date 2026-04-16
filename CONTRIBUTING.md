# Contributing

Thanks for helping improve Evey. This doc is the short version; see [DEV.md](DEV.md) for setup and [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for layering.

## Branching

- `master` is the trunk. It must always build and pass tests.
- Work on short-lived feature branches off `master`. Rebase before merging.
- Never merge stale branches — pull latest `master` first.

## Commit style

Conventional commits, scoped when useful:

- `feat(library): add last_scanned_at`
- `fix(player): surface silent catches`
- `refactor(auth): centralize HTTPAuthMiddleware`
- `test(user): cover CreateUser transactional rollback`
- `chore(deps): bump grpc-gateway`
- `docs(adr): add transactional unit ADR`

Keep messages concise. Sacrifice grammar for clarity.

## Pre-push checklist

Run locally before every push:

```sh
go vet ./...
go build ./...
pnpm -r typecheck
make test
make lint
```

For proto changes: `make proto-gen`.

## PR process

- Self-review your diff before merging.
- Keep PRs small and focused on one concern.
- Tests are mandatory for new behavior and for any bug fix.
- Update `docs/` and ADRs when decisions change.

## Testing pyramid

- **Unit (most)**: domain + app services against fakes. Fast, no IO.
- **Integration (some)**: adapter tests against real Postgres / Meilisearch via Docker.
- **E2E (few)**: user-facing flows. See [memory/e2e.md](memory/e2e.md).

Hexagonal layering means most logic is unit-testable — prefer that layer.

## Dependencies

Do not add dependencies without approval. For JS/TS use pnpm with `--save-exact`. For Go, pin in `go.mod` and run `go mod tidy`.

## Operations

Ops playbooks live in [docs/runbooks/](docs/runbooks/):

- [migration-runbook.md](docs/runbooks/migration-runbook.md) — schema migrations (apply, roll back, recover from dirty state, large-table patterns).
- [incident-runbook.md](docs/runbooks/incident-runbook.md) — first-response for server/auth/transcode/DB/Meili/cache/plugin outages.

Update these whenever you ship a change that alters ops behaviour (new env var, new failure mode, schema change pattern).

## References

- [DEV.md](DEV.md) — setup, build, run
- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)
- [docs/adr/](docs/adr/) — decision records
- [docs/runbooks/](docs/runbooks/) — operations playbooks
