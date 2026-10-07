#!/usr/bin/env bash
# Secrets scan for the pre-push gate: gitleaks over the outgoing commits.
#
# Pinned release, checksum verified, cached under ~/.cache so the gate needs no
# install step and no network once the binary is there. The rules are the
# gitleaks defaults plus .gitleaks.toml (project allow-list: test fixtures, the
# built-in TMDB read token).
#
#   scripts/secrets-scan.sh            commits BASE..HEAD (BASE as hygiene-guard picks it)
#   scripts/secrets-scan.sh --all      the whole history
#   HYGIENE_BASE=<rev> ...             explicit base (detached worktrees)
set -euo pipefail

VERSION=8.24.3
case "$(uname -m)" in
    x86_64)  ARCH=x64;   SHA=9991e0b2903da4c8f6122b5c3186448b927a5da4deef1fe45271c3793f4ee29c ;;
    aarch64) ARCH=arm64; SHA=5f2edbe1f49f7b920f9e06e90759947d3c5dfc16f752fb93aaafc17e9d14cf07 ;;
    *) echo "secrets-scan: unsupported architecture $(uname -m)" >&2; exit 2 ;;
esac

CACHE="${XDG_CACHE_HOME:-$HOME/.cache}/evey/gitleaks/$VERSION"
BIN="$CACHE/gitleaks"
if [ ! -x "$BIN" ]; then
    mkdir -p "$CACHE"
    TGZ="$CACHE/gitleaks.tar.gz"
    URL="https://github.com/gitleaks/gitleaks/releases/download/v$VERSION/gitleaks_${VERSION}_linux_${ARCH}.tar.gz"
    echo "→ secrets scan: fetching gitleaks $VERSION ($ARCH)"
    curl -fsSL --retry 3 -o "$TGZ" "$URL"
    echo "$SHA  $TGZ" | sha256sum -c --quiet - || { rm -f "$TGZ"; echo "secrets-scan: checksum mismatch for $URL" >&2; exit 2; }
    tar -xzf "$TGZ" -C "$CACHE" gitleaks && rm -f "$TGZ"
fi

ROOT="$(git rev-parse --show-toplevel)"
cd "$ROOT"

if [ "${1:-}" = "--all" ]; then
    echo "→ secrets scan: whole history"
    exec "$BIN" git --no-banner --redact --config .gitleaks.toml .
fi

# Same base as the hygiene guard: what is already published on this branch.
if [ -n "${HYGIENE_BASE:-}" ]; then
    BASE="$(git rev-parse --verify "${HYGIENE_BASE}^{commit}")"
else
    BRANCH="$(git rev-parse --abbrev-ref HEAD)"
    BASE=""
    if [ "$BRANCH" != "HEAD" ]; then BASE="$(git merge-base "origin/$BRANCH" HEAD 2>/dev/null || true)"; fi
    [ -n "$BASE" ] || BASE="$(git merge-base '@{u}' HEAD 2>/dev/null || true)"
    [ -n "$BASE" ] || BASE="$(git merge-base origin/master HEAD 2>/dev/null || true)"
fi

if [ -z "$BASE" ]; then
    # Nothing published yet (fresh remote): every commit is outgoing.
    echo "→ secrets scan: no published base, scanning the whole history"
    exec "$BIN" git --no-banner --redact --config .gitleaks.toml .
fi
HEAD="$(git rev-parse HEAD)"
if [ "$BASE" = "$HEAD" ]; then
    echo "→ secrets scan: nothing outgoing."
    exit 0
fi
echo "→ secrets scan: ${BASE:0:9}..${HEAD:0:9}"
exec "$BIN" git --no-banner --redact --config .gitleaks.toml --log-opts="$BASE..$HEAD" .
