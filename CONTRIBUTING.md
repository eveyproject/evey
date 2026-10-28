# Contributing

Thanks for helping improve Evey. This doc is the short version; see [DEV.md](DEV.md) for setup, [docs/CODEBASE_MAP.md](docs/CODEBASE_MAP.md) for the 30-minute codebase orientation (where things live + the five golden-path recipes), and [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for layering.

## Branching

- `master` is the trunk. It must always build and pass tests.
- Work on short-lived feature branches off `master`. Rebase before merging.
- Never merge stale branches - pull latest `master` first.

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
make pre-push
```

That is the single gate: vet, govulncheck, build, unit tests with `-race`, the
frontend suite, the gosec ratchet, lint, the architecture guards, the Postgres
suite and the transcode contract. It mirrors CI, so a green run here means a
green pipeline. `make hooks` wires it to `git push`; bypass one push
deliberately with `git push --no-verify`.

A push to `dev` or to a work branch runs `make pre-push-dev` instead: the same
gate without the long suites the pull request's CI reruns on the same commits,
and with the Go unit tests of the changed packages only.
A push to `master` runs the full gate.

For proto changes: `make proto-gen`.

## Contributor License Agreement

Evey is AGPLv3 and may also be offered under a commercial license. Keeping
that option requires the right to relicense every contribution, so every external
pull request needs a one-time acceptance of [CLA.md](CLA.md) - paste this in the
PR description:

```
I have read CLA.md and I accept it. Signed: <your full name>, <YYYY-MM-DD>.
```

You keep the copyright on your work. One acceptance covers everything you send
afterwards.

## PR process

- Self-review your diff before merging.
- Keep PRs small and focused on one concern.
- Tests are mandatory for new behavior and for any bug fix.
- Update `docs/` and ADRs when decisions change.

## Testing pyramid

- **Unit (most)**: domain + app services against fakes. Fast, no IO.
- **Integration (some)**: adapter tests against real Postgres / Meilisearch via Docker.
- **E2E (few)**: user-facing flows, Playwright specs under [`tests/e2e/`](tests/e2e/).

Hexagonal layering means most logic is unit-testable - prefer that layer.

## Dependencies

Do not add dependencies without approval. For JS/TS use pnpm with `--save-exact`. For Go, pin in `go.mod` and run `go mod tidy`.

## Operations

Ops playbooks live in [docs/runbooks/](docs/runbooks/):

- [migration-runbook.md](docs/runbooks/migration-runbook.md) - schema migrations (apply, roll back, recover from dirty state, large-table patterns).
- [incident-runbook.md](docs/runbooks/incident-runbook.md) - first-response for server/auth/transcode/DB/Meili/cache/plugin outages.

Update these whenever you ship a change that alters ops behaviour (new env var, new failure mode, schema change pattern).

## References

- [DEV.md](DEV.md) - setup, build, run
- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)
- [docs/adr/](docs/adr/) - decision records
- [docs/runbooks/](docs/runbooks/) - operations playbooks
