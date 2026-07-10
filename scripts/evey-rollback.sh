#!/usr/bin/env bash
#
# evey-rollback.sh -- roll the Evey stack back to a previously-built image.
#
# Runs ON THE HOST (it needs the Docker daemon; the evey container has no socket,
# by design). Docker keeps old images locally until they are pruned, so a
# rollback is just: stop, optionally restore the pre-migration DB, and bring the
# stack back up on the previous image tag.
#
# It is DRY-RUN by default: it prints the exact commands and does nothing. Add
# --apply to actually run them. For the current state + newest backup path, run
# `evey admin rollback` inside the container first.
#
# Usage:
#   scripts/evey-rollback.sh                          # show what it would do
#   scripts/evey-rollback.sh --tag 0.0.0 --apply      # roll back to evey:0.0.0
#   scripts/evey-rollback.sh --tag 0.0.0 --restore /data/backups/pre-migration-132-....sql.gz --apply
#
# Overridable via env (defaults suit a standard docker compose setup):
#   EVEY_IMAGE_REPO=evey      image repository to match (docker images '<repo>*')
#   EVEY_SERVICE=evey         compose service name
#   COMPOSE="docker compose"  compose command
#
set -euo pipefail

IMAGE_REPO="${EVEY_IMAGE_REPO:-evey}"
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
        -h|--help) sed -n '2,30p' "$0"; exit 0 ;;
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

echo "# stop the stack"
run $COMPOSE down
echo

if [ -n "$RESTORE" ]; then
    echo "# restore the pre-migration DB (only needed if the newer version migrated)"
    run $COMPOSE run --rm "$SERVICE" admin restore-backup "$RESTORE"
    echo
fi

echo "# bring the stack up on the previous image"
if [ "$APPLY" -eq 1 ]; then
    EVEY_IMAGE_TAG="$TAG" $COMPOSE up -d
else
    echo "  \$ EVEY_IMAGE_TAG=$TAG $COMPOSE up -d"
fi
echo

if [ "$APPLY" -eq 1 ]; then
    echo "Done. Verify with: $COMPOSE run --rm $SERVICE admin doctor"
else
    echo "Dry-run only. Re-run with --apply to execute."
fi
