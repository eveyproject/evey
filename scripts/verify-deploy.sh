#!/usr/bin/env bash
#
# verify-deploy.sh - "is that server running this commit?", answered over plain
# HTTP and nothing else.
#
# No SSH, no docker socket, no registry lookup: it reads the commit the SERVER
# itself reports on /healthz, which is stamped into the binary at link time
# (-ldflags -X main.commit=...). That is the only source that cannot drift from
# what is actually executing - an image tag, a compose file or a registry digest
# all describe what was *meant* to run.
#
# Usage:
#   scripts/verify-deploy.sh --url https://evey.example.tld
#   scripts/verify-deploy.sh --url https://evey.example.tld --commit <sha>
#   scripts/verify-deploy.sh --url https://evey.example.tld --ref origin/beta-0.0.1
#
# Exit codes - the caller can branch on them without parsing text:
#   0  MATCH     the server runs the expected commit
#   1  MISMATCH  the server runs a DIFFERENT commit (it names which)
#   2  UNKNOWN   the question cannot be answered: server unreachable, endpoint
#                missing, or the build was never stamped. Deliberately NOT 0:
#                "I could not check" is a deployment failure, not a pass.
#   3  USAGE     bad arguments / missing local git revision
set -uo pipefail

readonly EXIT_MATCH=0
readonly EXIT_MISMATCH=1
readonly EXIT_UNKNOWN=2
readonly EXIT_USAGE=3

URL="${EVEY_VERIFY_URL:-}"
EXPECTED=""
REF="HEAD"
TIMEOUT=15
QUIET=0

usage() {
  # Print the header comment block (everything between the shebang and the
  # first line of code), so the docs can never drift from a line count.
  awk 'NR==1{next} /^#/{sub(/^# ?/,""); print; next} {exit}' "$0"
  exit "$EXIT_USAGE"
}

while [ $# -gt 0 ]; do
  case "$1" in
    --url)     URL="${2:-}"; shift 2 || usage ;;
    --commit)  EXPECTED="${2:-}"; shift 2 || usage ;;
    --ref)     REF="${2:-}"; shift 2 || usage ;;
    --timeout) TIMEOUT="${2:-}"; shift 2 || usage ;;
    --quiet)   QUIET=1; shift ;;
    -h|--help) usage ;;
    *) echo "verify-deploy: unknown argument '$1'" >&2; usage ;;
  esac
done

say() { [ "$QUIET" -eq 1 ] || printf '%s\n' "$*"; }

[ -n "$URL" ] || { echo "verify-deploy: --url is required (or set EVEY_VERIFY_URL)" >&2; usage; }
command -v curl >/dev/null 2>&1 || { echo "verify-deploy: curl is required" >&2; exit "$EXIT_USAGE"; }

# ── What we expect ────────────────────────────────────────────────────────────
# Explicit --commit wins; otherwise resolve a local git revision. Refusing to
# guess when git is unavailable is the point: comparing against an empty string
# would report a spurious MATCH the moment the server also failed to answer.
if [ -z "$EXPECTED" ]; then
  EXPECTED="$(git rev-parse "$REF" 2>/dev/null || true)"
fi
if [ -z "$EXPECTED" ]; then
  echo "verify-deploy: could not resolve '$REF' locally - pass --commit <sha>" >&2
  exit "$EXIT_USAGE"
fi
EXPECTED="$(printf '%s' "$EXPECTED" | tr '[:upper:]' '[:lower:]')"

# ── What the server says ──────────────────────────────────────────────────────
HEALTH_URL="${URL%/}/healthz"
BODY="$(curl -sS --max-time "$TIMEOUT" -w '\n%{http_code}' "$HEALTH_URL" 2>/dev/null)" || {
  say "UNKNOWN  - $HEALTH_URL is unreachable"
  exit "$EXIT_UNKNOWN"
}
STATUS="$(printf '%s' "$BODY" | tail -n1)"
PAYLOAD="$(printf '%s' "$BODY" | sed '$d')"

# 503 is a DEGRADED but honest server (healthz answers 503 when the DB ping
# fails) - it still reports its build, so the identity question is answerable.
case "$STATUS" in
  200|503) : ;;
  *)
    say "UNKNOWN  - $HEALTH_URL returned HTTP $STATUS"
    exit "$EXIT_UNKNOWN"
    ;;
esac

# Field extraction without a jq dependency: /healthz is a flat, server-generated
# object, so a scoped regex is sufficient and keeps this runnable on a bare NAS.
json_field() {
  printf '%s' "$PAYLOAD" \
    | tr -d '\n' \
    | grep -o "\"$1\"[[:space:]]*:[[:space:]]*\"[^\"]*\"" \
    | head -n1 \
    | sed 's/.*:[[:space:]]*"\(.*\)"$/\1/'
}

REPORTED="$(json_field commit | tr '[:upper:]' '[:lower:]')"
BUILT_AT="$(json_field build_date)"
VERSION="$(json_field version)"

if [ -z "$REPORTED" ]; then
  say "UNKNOWN  - $HEALTH_URL has no \"commit\" field: that server predates commit stamping"
  say "           (version=${VERSION:-?}) - redeploy from a build that carries -X main.commit"
  exit "$EXIT_UNKNOWN"
fi
if [ "$REPORTED" = "unknown" ]; then
  say "UNKNOWN  - the server reports commit=\"unknown\": its image was built without a COMMIT build-arg"
  say "           (version=${VERSION:-?}) - rebuild with --build-arg COMMIT=\$(git rev-parse HEAD)"
  exit "$EXIT_UNKNOWN"
fi

# ── Verdict ───────────────────────────────────────────────────────────────────
# Compare on the shorter of the two, so a short SHA passed by hand still gives a
# definite answer instead of a false MISMATCH against the server's full one.
n=${#REPORTED}
[ "${#EXPECTED}" -lt "$n" ] && n=${#EXPECTED}
if [ "$n" -lt 7 ]; then
  say "UNKNOWN  - refusing to compare on fewer than 7 hex chars (server='$REPORTED', expected='$EXPECTED')"
  exit "$EXIT_UNKNOWN"
fi

if [ "${REPORTED:0:$n}" = "${EXPECTED:0:$n}" ]; then
  say "MATCH    - $URL runs ${REPORTED:0:12} (version=${VERSION:-?}, built ${BUILT_AT:-?})"
  exit "$EXIT_MATCH"
fi

say "MISMATCH - $URL runs ${REPORTED:0:12} (version=${VERSION:-?}, built ${BUILT_AT:-?})"
say "           expected ${EXPECTED:0:12}"
exit "$EXIT_MISMATCH"
