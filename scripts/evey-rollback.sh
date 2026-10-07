#!/usr/bin/env bash
#
# evey-rollback.sh -- roll the Evey stack back to a previously-built image.
#
# Runs ON THE HOST (it needs the Docker daemon; the evey container has no socket,
# by design). Docker keeps old images locally until they are pruned, so a
# rollback is just: relaunch the stack on the previous image tag, optionally
# restoring the pre-migration DB first.
#
# The tag is handed to compose through EVEY_VERSION, which is the variable
# docker-compose.yml interpolates (`image: eveyproject/evey:${EVEY_VERSION:-latest}`).
# It used to export EVEY_IMAGE_TAG, which compose reads nowhere: `--apply` tore
# the stack down and brought it straight back up on the version being rolled
# back FROM, while printing "Done.". internal/contractguard now fails the build
# if the two names drift apart again.
#
# Service interruption: without --restore there is no `down` at all - `up -d`
# recreates only the service whose image changed, so postgres and the reverse
# proxy stay up and the outage is the single container restart that swapping a
# binary requires. With --restore the server must be stopped while the database
# is rewritten under it, so that path stops the evey service (only) first.
#
# It is DRY-RUN by default: it prints the exact commands and does nothing. Add
# --apply to actually run them. For the current state + newest backup path, run
# `evey admin rollback` inside the container first.
#
# Usage:
#   scripts/evey-rollback.sh                          # show what it would do
#   scripts/evey-rollback.sh --tag 0.0.0 --apply      # roll back to eveyproject/evey:0.0.0
#   scripts/evey-rollback.sh --tag 0.0.0 --restore /data/backups/pre-migration-132-....sql.gz --apply
#
# Overridable via env (defaults suit a standard docker compose setup):
#   EVEY_IMAGE_REPO=eveyproject/evey   image repository to match (docker images '<repo>*')
#   EVEY_SERVICE=evey               compose service name
#   COMPOSE="docker compose"        compose command
#
set -euo pipefail

IMAGE_REPO="${EVEY_IMAGE_REPO:-eveyproject/evey}"
SERVICE="${EVEY_SERVICE:-evey}"
COMPOSE="${COMPOSE:-docker compose}"

TAG=""
RESTORE=""
APPLY=0
while [ $# -gt 0 ]; do
    case "$1" in
        --tag)     TAG="${2:-}"; shift 2 ;;
        --restore) RESTORE="${2:-}"; shift 2 ;;
        --apply)   APPLY=1; shift ;;
        -h|--help) sed -n '2,36p' "$0"; exit 0 ;;
        *) echo "unknown arg: $1" >&2; exit 2 ;;
    esac
done

echo "Evey images cached locally (newest first):"
docker images "${IMAGE_REPO}*" --format '  {{.Repository}}:{{.Tag}}\t{{.CreatedAt}}' \
    | sort -k2 -r || true
echo

# If no tag was given, offer the second-newest tag as the likely previous build.
if [ -z "$TAG" ]; then
    TAG=$(docker images "${IMAGE_REPO}" --format '{{.Tag}}' | sed -n '2p' || true)
    if [ -n "$TAG" ]; then
        echo "No --tag given; guessing the previous tag: ${IMAGE_REPO}:${TAG}"
    else
        echo "No --tag given and no previous tag found. Re-run with --tag <tag>." >&2
        exit 1
    fi
    echo
fi

run() {
    echo "  \$ $*"
    if [ "$APPLY" -eq 1 ]; then "$@"; fi
}

echo "Rollback plan (${IMAGE_REPO}:${TAG})$( [ "$APPLY" -eq 1 ] || echo '  [DRY-RUN -- add --apply to execute]' ):"
echo

if [ -n "$RESTORE" ]; then
    echo "# stop the server (postgres stays up: the restore needs it)"
    run $COMPOSE stop "$SERVICE"
    echo
    # The tag is pinned here too: a one-shot `run` started from the compose
    # file would otherwise pull in the very build being rolled back.
    echo "# restore the pre-migration DB (only needed if the newer version migrated)"
    run env EVEY_VERSION="$TAG" $COMPOSE run --rm "$SERVICE" admin restore-backup "$RESTORE"
    echo
fi

# No `down`: `up -d` recreates only the service whose image changed, so the
# database and the reverse proxy are never stopped.
echo "# bring the stack up on the previous image"
if [ "$APPLY" -eq 1 ]; then
    EVEY_VERSION="$TAG" $COMPOSE up -d
else
    echo "  \$ EVEY_VERSION=$TAG $COMPOSE up -d"
fi
echo

if [ "$APPLY" -eq 1 ]; then
    echo "Done. Verify with: $COMPOSE run --rm $SERVICE admin doctor"
else
    echo "Dry-run only. Re-run with --apply to execute."
fi
