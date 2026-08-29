#!/usr/bin/env bash
#
# evey-update.sh -- update a docker compose Evey install the same way every
# time: state, backup, pull, up, wait for health, verify, and the exact
# rollback line if it goes wrong. The twin of evey-rollback.sh.
#
# Runs ON THE HOST next to the compose file (it needs the Docker daemon; the
# evey container has no socket, by design). Works for every documented layout:
# the single container with the embedded database, or Postgres/Meilisearch as
# sibling services - the backup runs inside the evey container with the
# connection the server itself uses, so nothing here knows which one you have.
#
# It is DRY-RUN by default: it prints the exact commands and does nothing.
# Add --apply to actually run them.
#
# Usage:
#   scripts/evey-update.sh                          # show the plan
#   scripts/evey-update.sh --apply                  # update to EVEY_VERSION from .env
#   scripts/evey-update.sh --tag sha-1a2b3c4 --apply   # pin this run to one tag
#   scripts/evey-update.sh --commit <sha> --apply   # and verify the server reports it
#
# Options:
#   --tag <tag>       image tag for this run (default: EVEY_VERSION from .env, else latest);
#                     printed at the end so you can pin it in .env
#   --commit <sha>    fail (exit 3) if the updated server does not report this commit
#   --skip-backup     do not take a pre-update dump (the server still dumps before a migration)
#   --timeout <s>     how long to wait for the container to become healthy (default 600;
#                     a schema migration on a large catalogue can take minutes)
#   --apply           execute instead of printing
#
# Exit codes:
#   0 updated and healthy (or dry-run printed)
#   1 the new container never became healthy - the rollback line was printed
#   2 usage / precondition (no compose file, backup failed)
#   3 healthy but the server reports a commit other than --commit
#
# Overridable via env (defaults suit a standard docker compose setup):
#   EVEY_SERVICE=evey               compose service name
#   COMPOSE="docker compose"        compose command
#
set -euo pipefail
SERVICE="${EVEY_SERVICE:-evey}"
COMPOSE="${COMPOSE:-docker compose}"

TAG=""
WANT_COMMIT=""
SKIP_BACKUP=0
TIMEOUT=600
APPLY=0
while [ $# -gt 0 ]; do
    case "$1" in
        --tag)         TAG="${2:-}"; shift 2 ;;
        --commit)      WANT_COMMIT="${2:-}"; shift 2 ;;
        --skip-backup) SKIP_BACKUP=1; shift ;;
        --timeout)     TIMEOUT="${2:-}"; shift 2 ;;
        --apply)       APPLY=1; shift ;;
        -h|--help)     sed -n '2,40p' "$0"; exit 0 ;;
        *) echo "unknown arg: $1" >&2; exit 2 ;;
    esac
done
case "$TIMEOUT" in ''|*[!0-9]*) echo "--timeout must be a number of seconds" >&2; exit 2 ;; esac

if [ ! -f docker-compose.yml ] && [ ! -f compose.yml ] && [ ! -f compose.yaml ]; then
    echo "no compose file in $(pwd) - run this next to your Evey docker-compose.yml" >&2
    exit 2
fi

# The tag compose will actually launch: --tag, else .env, else the image
# default. Read from .env by hand so the plan is honest even in dry-run.
if [ -z "$TAG" ]; then
    if [ -f .env ]; then
        TAG=$(sed -n 's/^[[:space:]]*EVEY_VERSION=[[:space:]]*//p' .env | tail -n1 | tr -d '"'"'" || true)
    fi
    TAG="${TAG:-latest}"
fi

run() {
    echo "  \$ $*"
    if [ "$APPLY" -eq 1 ]; then "$@"; fi
}

container_id() { $COMPOSE ps -q "$SERVICE" 2>/dev/null | head -n1; }

echo "Evey update plan (EVEY_VERSION=$TAG)$( [ "$APPLY" -eq 1 ] || echo '  [DRY-RUN -- add --apply to execute]' ):"
echo

# ── 1. Where we are ─────────────────────────────────────────────────────
# Read-only: version, commit, schema, newest backup. Printed now because it is
# what a rollback needs to know, and it is easiest to read BEFORE the update.
PREV_TAG=""
CID=$(container_id || true)
if [ -n "$CID" ]; then
    PREV_TAG=$(docker inspect --format '{{.Config.Image}}' "$CID" 2>/dev/null | sed 's/.*://' || true)
    echo "# current state (read-only)"
    if [ "$APPLY" -eq 1 ]; then
        $COMPOSE exec -T "$SERVICE" evey admin rollback 2>/dev/null | sed 's/^/  /' || echo "  (evey admin rollback unavailable - container not ready?)"
    else
        echo "  \$ $COMPOSE exec -T $SERVICE evey admin rollback"
    fi
    echo "  running image tag: ${PREV_TAG:-?}"
    echo
else
    echo "# no running $SERVICE container: first install, or the stack is down"
    echo
fi

# ── 2. Backup ───────────────────────────────────────────────────────────
# Same dump the server takes before a migration (plain SQL, no owner, gzip,
# .sql.gz - what `evey admin restore-backup` accepts), taken with the server's
# own connection so it works for the embedded database (password file) and
# for a sibling Postgres (env) alike. `exec` runs as root, so the file is
# handed to the backups dir owner at 0600 like the server's own dumps.
# Skipped when nothing is running.
BACKUP=""
if [ "$SKIP_BACKUP" -eq 0 ] && [ -n "$CID" ]; then
    STAMP=$(date -u +%Y%m%dT%H%M%SZ)
    BACKUP="/data/backups/pre-update-${STAMP}.sql.gz"
    echo "# pre-update database dump -> $BACKUP (inside ./evey-data/backups)"
    # shellcheck disable=SC2016  # expanded inside the container on purpose
    DUMP_CMD='PGPASSWORD="${EVEY_DB_PASSWORD:-$(cat "${EVEY_DB_PASSWORD_FILE:-/data/keys/postgres.password}" 2>/dev/null)}" pg_dump --host "${EVEY_DB_HOST:-127.0.0.1}" --port "${EVEY_DB_PORT:-5432}" --username "${EVEY_DB_USER:-evey}" --dbname "${EVEY_DB_NAME:-evey}" --no-password --format=plain --no-owner --no-privileges | gzip > "$1" && gunzip -t "$1" && chmod 600 "$1" && chown --reference=/data/backups "$1" && ls -l "$1"'
    echo "  \$ $COMPOSE exec -T $SERVICE sh -c '<pg_dump | gzip> $BACKUP'"
    if [ "$APPLY" -eq 1 ]; then
        if ! $COMPOSE exec -T "$SERVICE" sh -c "$DUMP_CMD" _ "$BACKUP" | sed 's/^/  /'; then
            echo "backup FAILED - not updating (use --skip-backup to override, at your own risk)" >&2
            exit 2
        fi
    fi
    echo
fi

# ── 3. Pull + up ────────────────────────────────────────────────────────
# `up -d` recreates only the service whose image changed: a sibling database
# or proxy is never restarted. With the embedded database the container
# itself stops Postgres cleanly after the server (stop_grace_period matters).
echo "# pull and relaunch on $TAG"
if [ "$APPLY" -eq 1 ]; then
    if ! EVEY_VERSION="$TAG" $COMPOSE pull "$SERVICE"; then
        # A registry that is down must not block a host that already holds
        # the image (an offline NAS, a locally built tag); a tag that exists
        # nowhere must. `compose config` resolves the exact image reference.
        IMAGE=$(EVEY_VERSION="$TAG" $COMPOSE config --images "$SERVICE" 2>/dev/null | head -n1 || true)
        if [ -n "$IMAGE" ] && docker image inspect "$IMAGE" >/dev/null 2>&1; then
            echo "  pull failed but $IMAGE is present locally - continuing with it" >&2
        else
            echo "pull failed and ${IMAGE:-the image} is not available locally - not updating" >&2
            exit 2
        fi
    fi
    EVEY_VERSION="$TAG" $COMPOSE up -d
else
    echo "  \$ EVEY_VERSION=$TAG $COMPOSE pull $SERVICE"
    echo "  \$ EVEY_VERSION=$TAG $COMPOSE up -d"
fi
echo

# ── 4. Wait for health ──────────────────────────────────────────────────
# The image's own HEALTHCHECK (/healthz) is the signal, so no port needs to
# be published: it is what compose uses for depends_on as well. Failing
# inside the window is expected during a migration; only `unhealthy` or an
# exited container ends the wait early.
rollback_hint() {
    echo
    echo "To roll back:"
    echo "  scripts/evey-rollback.sh --tag ${PREV_TAG:-<previous tag>}${BACKUP:+ --restore $BACKUP} --apply"
    echo "  (--restore only if the new version migrated the schema; \`evey admin rollback\` says so)"
}

echo "# wait up to ${TIMEOUT}s for $SERVICE to report healthy"
if [ "$APPLY" -eq 1 ]; then
    CID=$(container_id || true)
    if [ -z "$CID" ]; then
        echo "no $SERVICE container after up -d - check \`$COMPOSE logs $SERVICE\`" >&2
        rollback_hint
        exit 1
    fi
    waited=0
    while :; do
        STATUS=$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$CID" 2>/dev/null || echo missing)
        case "$STATUS" in
            healthy) echo "  healthy after ${waited}s"; break ;;
            unhealthy|exited|dead|missing)
                echo "  container is $STATUS after ${waited}s" >&2
                $COMPOSE logs --tail=40 "$SERVICE" 2>/dev/null | sed 's/^/  /' || true
                rollback_hint
                exit 1 ;;
        esac
        if [ "$waited" -ge "$TIMEOUT" ]; then
            echo "  still $STATUS after ${TIMEOUT}s - a long migration, or a boot that hangs; watch \`$COMPOSE logs -f $SERVICE\`" >&2
            rollback_hint
            exit 1
        fi
        sleep 5; waited=$((waited + 5))
    done
else
    echo "  \$ docker inspect --format '{{.State.Health.Status}}' \$($COMPOSE ps -q $SERVICE)   # until healthy"
fi
echo

# ── 5. Verify ───────────────────────────────────────────────────────────
# What is RUNNING, from the binary itself - never from a tag or a digest.
echo "# what the server says it is"
if [ "$APPLY" -eq 1 ]; then
    VERSION_OUT=$($COMPOSE exec -T "$SERVICE" evey version 2>/dev/null || true)
    echo "$VERSION_OUT" | sed 's/^/  /'
    if [ -n "$WANT_COMMIT" ]; then
        GOT=$(echo "$VERSION_OUT" | awk '/^commit/{print $2}' | tr '[:upper:]' '[:lower:]')
        WANT=$(echo "$WANT_COMMIT" | tr '[:upper:]' '[:lower:]')
        case "$GOT" in
            "$WANT"*|"${WANT:0:7}"*) echo "  commit matches --commit" ;;
            *) echo "  MISMATCH: server reports '${GOT:-?}', expected '$WANT'" >&2; rollback_hint; exit 3 ;;
        esac
    fi
    echo
    echo "Done. Pin what is now running: EVEY_VERSION=$TAG in .env."
    rollback_hint
else
    echo "  \$ $COMPOSE exec -T $SERVICE evey version"
    echo
    echo "Dry-run only. Re-run with --apply to execute."
fi
