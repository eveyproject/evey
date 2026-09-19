#!/usr/bin/env bash
# verify-commit.sh REF - check a commit the way a fresh clone would see it.
#
# The working tree lies about what a commit contains: an uncommitted file is on
# disk, so it resolves for every check that reads the tree, and the break only
# surfaces in `docker build` or CI. That happened twice in one day on
# 2026-09-19. So this runs against a DETACHED worktree of REF, created outside
# the repo, containing nothing but what REF actually committed.
#
#     make verify-commit REF=HEAD
#     make verify-commit REF=93443cac4 FRONTEND=1     # + clean pnpm install
#     KEEP=1 ...                                      # keep the worktree to poke at
#
# The steps run from INSIDE that worktree, wrapped in a gate receipt, so the
# receipt pins the verified sha and a clean tree - not the state of the checkout
# you happened to be sitting in.
#
# What it covers, and what it does not: the host-side guards, buf generate, and
# the Go build and unit suite, all against REF alone. The gosec/lint/proto steps
# of `make pre-push` are NOT reproduced - they exec into the long-lived
# evey-server container, which is bound to the MAIN tree, and pointing them at a
# worktree would silently check the wrong files. This is a clean-checkout check,
# not a second gate; `make gate` remains the gate.

set -euo pipefail

GO_CI_IMAGE="${GO_CI_IMAGE:-golang:1.27-trixie}"
NODE_CI_IMAGE="${NODE_CI_IMAGE:-node:22}"
GO_UNIT_PKGS="${GO_UNIT_PKGS:-./cmd/... ./internal/... ./tests/...}"
UNIT="${UNIT:-1}"
FRONTEND="${FRONTEND:-0}"
KEEP="${KEEP:-0}"

# ---------------------------------------------------------------- inner phase
# Runs with the worktree as cwd, under the receipt. Everything it touches is REF.
if [ "${VERIFY_INNER:-0}" = "1" ]; then
    WT="$(pwd)"
    SHORT="$(git rev-parse --short=9 HEAD)"

    echo "→ [1/5] hygiene guard (at $SHORT)"
    python3 scripts/hygiene-guard.py --selftest
    python3 scripts/hygiene-guard.py

    echo "→ [2/5] clean-checkout guard (at $SHORT)"
    python3 scripts/clean-checkout-guard.py

    # gen/ is generated, never committed: without this the Go steps below would
    # fail for a reason that has nothing to do with REF. Explicit failure when
    # buf is missing - a verification that skips its own prerequisite in silence
    # is worse than no verification.
    echo "→ [3/5] buf generate (gen/ is not committed)"
    BUF="${BUF:-$HOME/go/bin/buf}"
    [ -x "$BUF" ] || { echo "verify-commit: buf not found at $BUF - install it or set BUF=" >&2; exit 1; }
    PATH="$HOME/go/bin:$PATH" "$BUF" generate --template buf.gen.local.yaml

    mkdir -p "$HOME/.cache/evey-gocache" "$HOME/go/pkg/mod"
    docker_go() {
        docker run --rm --user "$(id -u):$(id -g)" \
            -v "$WT":/app -w /app \
            -v "$HOME/go/pkg/mod":/go/pkg/mod \
            -v "$HOME/.cache/evey-gocache":/gocache -e GOCACHE=/gocache -e HOME=/tmp \
            "$GO_CI_IMAGE" sh -c "$1"
    }

    echo "→ [4/5] go vet + go build (throwaway $GO_CI_IMAGE, non-root, as CI)"
    docker_go 'go vet $(go list ./... | grep -v /gen/) && go build ./...'

    if [ "$UNIT" = "1" ]; then
        echo "→ [5/5] go test -race (unit, excl. postgres+gen)"
        docker_go "go test \$(go list $GO_UNIT_PKGS | grep -v 'adapters/driven/postgres' | grep -v /gen/) -timeout=600s -race"
    else
        echo "→ [5/5] go test SKIPPED (UNIT=0)"
    fi

    if [ "$FRONTEND" = "1" ]; then
        echo "→ [bonus] pnpm install --frozen-lockfile (what a clean clone fails on first)"
        docker run --rm --user "$(id -u):$(id -g)" \
            -v "$WT/frontend":/app -w /app -e HOME=/tmp -e CI=1 \
            "$NODE_CI_IMAGE" sh -c 'corepack enable >/dev/null 2>&1; pnpm install --frozen-lockfile --ignore-scripts'
    fi

    echo "✅ clean checkout of $SHORT guards, generates, vets and builds$([ "$UNIT" = 1 ] && echo ', passes unit -race')$([ "$FRONTEND" = 1 ] && echo ', installs')."
    exit 0
fi

# ---------------------------------------------------------------- outer phase
REF="${REF:-${1:-}}"
[ -n "$REF" ] || { echo "verify-commit: REF is required (make verify-commit REF=HEAD)" >&2; exit 2; }

# The driver comes from the tree you invoked it in; the guards it runs come from
# the commit under test (cwd is the worktree). That split is deliberate: you
# choose the procedure, REF supplies the thing being checked.
SELF="$(cd "$(dirname "$0")" && pwd)/$(basename "$0")"

ROOT="$(git rev-parse --show-toplevel)"
SHA="$(git rev-parse --verify "$REF^{commit}")"
SHORT="${SHA:0:9}"
SLUG="$(echo "$ROOT" | sed 's/[^A-Za-z0-9]/-/g')"
WT="$HOME/.claude/projects/$SLUG/verify/$SHORT"

cleanup() {
    if [ "$KEEP" = "1" ]; then
        echo "→ worktree kept at $WT (remove with: git worktree remove --force '$WT')"
    else
        git -C "$ROOT" worktree remove --force "$WT" >/dev/null 2>&1 || true
    fi
}
trap cleanup EXIT

echo "→ verify-commit $SHORT ($(git log -1 --format=%s "$SHA" | cut -c1-70))"
git -C "$ROOT" worktree remove --force "$WT" >/dev/null 2>&1 || true
mkdir -p "$(dirname "$WT")"
git -C "$ROOT" worktree add --detach "$WT" "$SHA" >/dev/null

# The worktree is detached, so the guard cannot work out what is already
# published on the branch. This tree can: compute the ratchet base here.
BRANCH="$(git -C "$ROOT" rev-parse --abbrev-ref HEAD)"
HYGIENE_BASE="$(git -C "$ROOT" merge-base "origin/$BRANCH" "$SHA" 2>/dev/null \
    || git -C "$ROOT" merge-base origin/master "$SHA" 2>/dev/null \
    || git -C "$ROOT" rev-parse "$SHA~1")"

cd "$WT"
# Not `exec`: the EXIT trap above is what removes the worktree.
set +e
python3 scripts/gate-receipt.py --label "verify-$SHORT" run -- \
    env VERIFY_INNER=1 HYGIENE_BASE="$HYGIENE_BASE" \
        GO_CI_IMAGE="$GO_CI_IMAGE" NODE_CI_IMAGE="$NODE_CI_IMAGE" \
        GO_UNIT_PKGS="$GO_UNIT_PKGS" UNIT="$UNIT" FRONTEND="$FRONTEND" \
    bash "$SELF"
status=$?
exit "$status"
