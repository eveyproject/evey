# Release runbook

How a version of Evey goes from a branch to something an operator can pin.
Written before the first tagged release so the first one follows a procedure
instead of inventing it.

## What CI does with a tag (read this before tagging)

`.github/workflows/ci.yml` builds and publishes images on every push and on
every `v*.*.*` tag, in three plugin-runtime variants (`` (lean), `-nodejs`,
`-full`), to `docker.io/gravityz/evey` and `ghcr.io/evey-project/evey`:

| You push | Image tags published (each also `-nodejs` / `-full`) |
|---|---|
| a branch (e.g. `beta-0.0.1`) | `beta-0.0.1`, `sha-<short>` |
| tag `v0.2.0-beta.1` (pre-release: has a hyphen) | `0.2.0-beta.1`, `sha-<short>` |
| tag `v0.2.0` (stable) | `0.2.0`, `0.2`, `0`, `latest`, `sha-<short>` |

Three consequences:

- **A branch tag moves.** `beta-0.0.1-full` points at whatever was pushed
  last; two `docker compose pull` a day apart may run different code, and a
  rollback "to `beta-0.0.1-full`" goes nowhere. Operators must pin a
  `sha-<short>` tag (or a release tag), never a branch name - that is what
  `scripts/evey-update.sh` prints at the end for them to put in `.env`.
- **A pre-release never touches `latest`.** `latest` means the newest stable
  release; a closed-beta cut is invisible to anyone tracking it. (A pre-release
  also gets no `X.Y` / `X` tags: those are stable-only.)
- **Multi-arch on tags only.** A `v*.*.*` tag (stable or pre-release) builds
  amd64 + arm64; a branch push builds amd64 only, unless its commit message
  carries `[arm64]` or the workflow is dispatched by hand. An ARM operator pins
  a release tag.

The commit is stamped into the binary (`evey version`, `/healthz`) and into
the image labels (`org.opencontainers.image.revision`), so "which code is
this?" is always answerable without trusting a tag.

## Cutting a pre-release (closed beta)

1. **Gates green on the exact commit**: `make pre-push` locally. CI runs the
   same on push.
2. **Proven on a real install first.** The commit must already have run on
   the reference deployment as a `sha-<short>` tag, updated with
   `scripts/evey-update.sh --tag sha-<short> --apply` and verified with
   `--commit <sha>`. A tag is a promise to other operators; make it after the
   proof, not before.
3. **`CHANGELOG.md`**: move the `[Unreleased]` entries under the new version
   with the date. Keep a Changelog format, user-facing wording only.
4. **Tag from that commit**, annotated, and push the tag only:
   ```bash
   git tag -a v0.2.0-beta.1 -m "v0.2.0-beta.1"
   git push origin v0.2.0-beta.1
   ```
5. **Watch CI** publish `0.2.0-beta.1` (+ variants). Then, on the reference
   deployment, `scripts/evey-update.sh --tag 0.2.0-beta.1 --apply` - pinning
   the release tag replaces the `sha-` pin.
6. **Announce the tag** with its changelog section. Operators pin
   `gravityz/evey:0.2.0-beta.1` (or `-full`) on their `image:` line.

Pre-release tags are cheap: `-beta.2`, `-beta.3` as needed. What they are not
is retractable - a published tag stays published. To undo a bad cut, cut the
next one; never re-point a tag.

## Cutting a stable release

Same steps, tag `vX.Y.Z` without a suffix. One thing to check that a
pre-release does not need: the Postgres major the image embeds must be recorded
in the changelog, because an operator on the embedded database has to follow the
upgrade section of [installation.md](../installation.md) when it changes.

## Rolling back a release

Images stay in the registry. An operator rolls back with
`scripts/evey-rollback.sh --tag <previous> [--restore <pre-update dump>] --apply`
(the update script prints that exact line). Nothing on the release side needs
undoing; publish a fixed `-beta.N+1` when the fix lands.
