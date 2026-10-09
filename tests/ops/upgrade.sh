#!/usr/bin/env bash
#
# upgrade.sh - update and rollback, the way an operator does it: a compose pile on
# the previous published image, then scripts/evey-update.sh to the candidate, then
# scripts/evey-rollback.sh --restore back. Accounts, catalogue and playback are
# checked after each move. Pulls one image and boots three times: 2-3 min.
#
# Usage:
#   tests/ops/upgrade.sh                                  # FROM=beta-0.0.1-full -> IMG=ops-local
#   FROM=eveyproject/evey:sha-1a2b3c4-full IMG=eveyproject/evey:ops-local tests/ops/upgrade.sh
#
# Env: FROM image to start on · IMG candidate (any name: it is retagged under
#      eveyproject/evey, the repository the compose file and the scripts expect) ·
#      PORT host port (19094) · OPS_OUT logs dir · KEEP=1 leaves the pile up.
#
set -u
FROM="${FROM:-eveyproject/evey:beta-0.0.1-full}"
IMG="${IMG:-eveyproject/evey:ops-local}"
PORT="${PORT:-19094}"
OUT="${OPS_OUT:-$(cd "$(dirname "$0")" && pwd)/test-results-upgrade}"
KEEP="${KEEP:-0}"
B="http://127.0.0.1:$PORT"
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
# shellcheck source=tests/ops/lib.sh
. "$(cd "$(dirname "$0")" && pwd)/lib.sh"

FROM_TAG="${FROM##*:}"
CAND_TAG="ops-candidate"
docker image inspect "$FROM" >/dev/null 2>&1 || docker pull "$FROM" >/dev/null || { echo "cannot pull $FROM" >&2; exit 2; }
docker image inspect "$IMG" >/dev/null 2>&1 || { echo "candidate $IMG is not present locally (make ops-image)" >&2; exit 2; }
docker tag "$IMG" "eveyproject/evey:$CAND_TAG"
CAND_COMMIT=$(docker image inspect --format '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$IMG" 2>/dev/null)

mkdir -p "$OUT"; : > "$OUT/run.log"
R=$(mktemp -d "${TMPDIR:-/tmp}/evey-ops-upgrade.XXXXXX"); mkdir -p "$R/data" "$R/media"
cleanup() {
    (cd "$R" && docker compose logs --no-color > "$OUT/container.log" 2>&1; cat data/logs/*.log > "$OUT/server.log" 2>/dev/null) || true
    if [ "$KEEP" = "1" ]; then echo "kept: cd $R && docker compose ps"; else (cd "$R" && docker compose down -v >/dev/null 2>&1); fi
}
trap cleanup EXIT
START=$(now_ms)

# The pile mirrors the documented single-container layout; the image tag goes through
# EVEY_VERSION because that is the variable the two scripts drive.
cat > "$R/docker-compose.yml" <<YML
services:
  evey:
    image: eveyproject/evey:\${EVEY_VERSION:-$FROM_TAG}
    ports:
      - "127.0.0.1:$PORT:9091"
    environment:
      EVEY_TITLE_PROBE: "off"
    volumes:
      - ./data:/data
      - evey-cache:/cache
      - ./media:/media
    stop_grace_period: 60s
volumes:
  evey-cache:
YML
echo "from $FROM  candidate $IMG (${CAND_COMMIT:-commit unknown})  pile $R" | tee -a "$OUT/run.log"

t=$(now_ms)
make_clip "$R/media" "$FROM" 2>"$OUT/ffmpeg.err" || { report FAIL clip "$t" "$(head -c 200 "$OUT/ffmpeg.err")"; exit 1; }
(cd "$R" && docker compose up -d >/dev/null 2>&1)
wait_healthy "$B" 300 || { report FAIL boot_from "$t" "healthz never answered"; exit 1; }
FROM_COMMIT=$(commit_of "$B")
TOK=$(first_admin "$B"); LIB=$(create_library "$B" "$TOK"); scan "$B" "$TOK" "$LIB"
read -r N ID <<<"$(items "$B" "$TOK")"
[ "${N:-0}" = "1" ] && report PASS boot_from "$t" "commit=${FROM_COMMIT:0:9} items=$N" || { report FAIL boot_from "$t" "items=${N:-?}"; exit 1; }

check_after() { # NAME T0 WANT_COMMIT - login, catalogue, one transcoded segment
    local tok n id st code tries got
    got=$(commit_of "$B"); tok=$(login "$B"); read -r n id <<<"$(items "$B" "$tok")"
    st=$(stream_token "$B" "$tok" "$id"); read -r code tries <<<"$(first_segment "$B/api/v1/media/$id/hls" "$st" "&maxHeight=240&videoCodecs=h264" "$R")"
    if [ -n "$tok" ] && [ "${n:-0}" = "1" ] && [ "$code" = "200" ] && { [ -z "$3" ] || [ "$got" = "$3" ]; }; then
        report PASS "$1" "$2" "commit=${got:0:9} login ok items=$n segment http=$code"
    else report FAIL "$1" "$2" "commit=${got:0:9} want=${3:0:9} login=${tok:+ok}${tok:-failed} items=${n:-?} segment http=$code"; fi
}

# Update: the script dumps, pulls (falls back to the local tag), relaunches, waits for health.
t=$(now_ms)
(cd "$R" && timeout 240 "$ROOT/scripts/evey-update.sh" --tag "$CAND_TAG" ${CAND_COMMIT:+--commit "$CAND_COMMIT"} --apply > "$OUT/update.log" 2>&1); rc=$?
if [ $rc -ne 0 ]; then report FAIL update "$t" "evey-update.sh exit=$rc ($OUT/update.log)"; exit 1; fi
check_after update "$t" "$CAND_COMMIT"
BACKUP=$(ls -t "$R"/data/backups/pre-update-*.sql.gz 2>/dev/null | head -1)
[ -n "$BACKUP" ] && report PASS pre_update_dump "$t" "$(basename "$BACKUP") $(stat -c %s "$BACKUP") bytes" || report FAIL pre_update_dump "$t" "no pre-update dump in data/backups"

# Restore the pre-update dump with restore-backup, run by the candidate binary
# (server stopped during the restore, as evey-rollback.sh --restore does). The
# dump now carries --clean --if-exists, so the schema is dropped and rebuilt in
# one transaction; restore-backup verifies the dump is whole before dropping.
if [ -n "$BACKUP" ]; then
    t=$(now_ms)
    (cd "$R" && timeout 120 "$ROOT/scripts/evey-rollback.sh" --tag "$CAND_TAG" --restore "/data/backups/$(basename "$BACKUP")" --apply > "$OUT/restore.log" 2>&1); rc=$?
    wait_healthy "$B" 180 || true
    if [ $rc -eq 0 ] && [ "$(items "$B" "$(login "$B")" | cut -d' ' -f1)" = "1" ]; then
        report PASS restore_backup "$t" "clean restore over a live schema, items kept ($OUT/restore.log)"
    else
        report FAIL restore_backup "$t" "rc=$rc ($OUT/restore.log)"
    fi
fi

# Rollback, image only: the common case when nothing migrated. The previous
# binary comes back up on the same /data and keeps serving.
t=$(now_ms)
(cd "$R" && timeout 240 "$ROOT/scripts/evey-rollback.sh" --tag "$FROM_TAG" --apply > "$OUT/rollback.log" 2>&1); rc=$?
if [ $rc -ne 0 ]; then report FAIL rollback_image "$t" "evey-rollback.sh exit=$rc ($OUT/rollback.log)"; exit 1; fi
wait_healthy "$B" 300 || { report FAIL rollback_image "$t" "healthz never answered after rollback"; exit 1; }
check_after rollback_image "$t" "$FROM_COMMIT"


echo "total $(fmt "$(since "$START")") s  from=$FROM candidate=$IMG  failed=$FAILED  logs=$OUT" | tee -a "$OUT/run.log"
exit $FAILED
