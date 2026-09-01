# Runbooks

Operational playbooks for running Evey in production (or a serious home lab).

## Index

- [cli.md](cli.md) - `evey admin` operator CLI reference (migrate, reset-password, rotate-jwt-key, restore-backup, reindex-search) + standalone dev/QA binaries.
- [media-health.md](media-health.md) - the admin Media health page: what each verdict means (repairable / your call / info), what Repair all does and asks, acknowledging findings, why the first scan after an upgrade is long, backups and rollback.
- [migration-runbook.md](migration-runbook.md) - applying, rolling back, and recovering from database migrations.
- [incident-runbook.md](incident-runbook.md) - first-response diagnostics for common outages (server down, auth broken, transcode fail, DB/Meili issues, cache full, plugin crash, WSL git corruption).
- [release.md](release.md) - what CI publishes for a branch, a pre-release tag and a stable tag; how to cut one; why operators pin `sha-`/release tags, never a branch.
- `scripts/evey-update.sh` / `scripts/evey-rollback.sh` - host-side update and rollback, dry-run by default (see `docs/installation.md` § 4).

## When to use which

| Situation | Runbook |
|---|---|
| Looking up an `evey admin` subcommand or invocation pattern | `cli.md` |
| Media health page: a verdict you do not understand, Repair all, a scan that takes hours after an upgrade | `media-health.md` |
| `make migrate-up` / `make migrate-down` / `make migrate-version` | `cli.md` → migrate |
| Updating an install (any layout) | `scripts/evey-update.sh --apply` (`installation.md` § 4) |
| Deploying a new Evey version that changes schema | `migration-runbook.md` |
| Tagging a beta or a release | `release.md` |
| `schema_migrations.dirty = true` after a crashed upgrade | `migration-runbook.md` → Recovery |
| Planning a large `ALTER`, `DROP COLUMN`, or FK addition | `migration-runbook.md` → Special cases |
| `/healthz` failing, container restart loop, Caddy 502 | `incident-runbook.md` → §1 |
| Every login returns 401 | `incident-runbook.md` → §2 |
| Transcodes failing, playback broken | `incident-runbook.md` → §3, §6 |
| Postgres / Meilisearch outage | `incident-runbook.md` → §4, §5 |
| Plugin panic in logs | `incident-runbook.md` → §7 |
| WSL2 dev host: corrupt git objects | `incident-runbook.md` → §8 |

## Related docs

- `docs/configuration.md` - env vars and defaults.
- `docs/backups.md` - backup/restore configuration.
- `docs/transcoding.md` - ffmpeg and cache behaviour.
- `docs/reverse-proxy.md` - trusted proxies, forwarded headers.

## Contacts / escalation

TODO - fill in for your deployment:

| Role | Name | Contact | Scope |
|---|---|---|---|
| Primary on-call | _TODO_ | _TODO_ | _TODO_ |
| Secondary | _TODO_ | _TODO_ | _TODO_ |
| DBA / Postgres | _TODO_ | _TODO_ | _TODO_ |
| Plugin authors | _TODO_ | _TODO_ | _TODO_ |
