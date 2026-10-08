#!/usr/bin/env bash
#
# run.sh - the operations harness: boots the image as a user would and walks the
# scenarios an installation meets in real life, on one throwaway pile.
#
# Fresh install, library scan, direct play, parallel clients, forced transcode,
# cache disk full, library root unreadable, SIGKILL and restart. Every step
# prints PASS/FAIL with the measurement that decided it. Only HTTP talks to the
# server; docker is used to boot, break and inspect. ~10 s on a warm image.
#
# Usage:
#   tests/ops/run.sh                      # IMG defaults to eveyproject/evey:ops-local (make ops-image)
#   IMG=eveyproject/evey:beta-0.0.1-full tests/ops/run.sh
#
# Env: IMG image under test · PORT host port (19092) · OPS_OUT logs dir
#      (tests/ops/test-results) · KEEP=1 leaves the container running.
#
# Exit: 0 all green, 1 a scenario failed. An XFAIL (known defect, referenced
# inline) does not fail the run; an XPASS does, so the mark gets removed.
#
set -u

IMG="${IMG:-eveyproject/evey:ops-local}"
PORT="${PORT:-19092}"
NAME="${NAME:-evey-ops-$PORT}"
OUT="${OPS_OUT:-$(cd "$(dirname "$0")" && pwd)/test-results}"
KEEP="${KEEP:-0}"
B="http://127.0.0.1:$PORT"
# shellcheck source=tests/ops/lib.sh
. "$(cd "$(dirname "$0")" && pwd)/lib.sh"

cleanup() {
    docker exec "$NAME" sh -c 'cat /data/logs/*.log' > "$OUT/server.log" 2>/dev/null || true
    docker logs "$NAME" > "$OUT/container.log" 2>&1 || true
    if [ "$KEEP" = "1" ]; then echo "kept: docker logs $NAME; curl $B/healthz"; else docker rm -f "$NAME" >/dev/null 2>&1; fi
    chmod 755 "$R/media" 2>/dev/null || true
}
trap cleanup EXIT

mkdir -p "$OUT"; : > "$OUT/run.log"
R=$(mktemp -d "${TMPDIR:-/tmp}/evey-ops.XXXXXX"); mkdir -p "$R/data" "$R/media"
docker rm -f "$NAME" >/dev/null 2>&1
START=$(now_ms)
echo "image $IMG  pile $R" | tee -a "$OUT/run.log"

# The clip comes from the image's own ffmpeg: 30 s, two audio tracks, owned by
# the caller. Its mtime is pushed back so the scan does not wait for it to settle.
t=$(now_ms)
make_clip "$R/media" "$IMG" 2>"$OUT/ffmpeg.err" \
    && report PASS clip "$t" "$(stat -c %s "$R/media/$CLIP") bytes" \
    || { report FAIL clip "$t" "$(head -c 200 "$OUT/ffmpeg.err")"; exit 1; }

# Boot 1: empty /data, /cache on a 1 MiB tmpfs so the disk-full scenario can fill it.
t=$(now_ms)
docker run -d --name "$NAME" -p "127.0.0.1:$PORT:9091" -v "$R/data:/data" -v "$R/media:/media" \
    --tmpfs "/cache:size=1m,uid=$(id -u),gid=$(id -g)" --stop-timeout 60 -e EVEY_TITLE_PROBE=off "$IMG" >/dev/null
wait_healthy "$B"
COMMIT=$(commit_of "$B" | cut -c1-9)
SCHEMA=$(docker exec "$NAME" evey admin migrate version 2>/dev/null | tail -1)
if [ -n "$COMMIT" ]; then report PASS boot "$t" "commit=$COMMIT $SCHEMA"; else report FAIL boot "$t" "healthz never answered"; exit 1; fi

# First admin, one movie library, one scan (the scan call streams until done).
t=$(now_ms)
TOK=$(first_admin "$B")
LIB=$(create_library "$B" "$TOK")
scan "$B" "$TOK" "$LIB"; read -r N ID <<<"$(items "$B" "$TOK")"
if [ "${N:-0}" = "1" ]; then report PASS setup_scan "$t" "items=$N"; else report FAIL setup_scan "$t" "items=${N:-?} token=${TOK:0:6} lib=$LIB"; exit 1; fi

# Direct play: master, variant playlist, first segment.
t=$(now_ms)
ST=$(stream_token "$B" "$TOK" "$ID")
M="$B/api/v1/media/$ID/hls"
read -r code tries <<<"$(first_segment "$M" "$ST" "" "$R")"
[ "$code" = "200" ] && report PASS direct_play "$t" "segment $(stat -c %s "$R/seg") bytes" || report FAIL direct_play "$t" "http=$code"

# Eight clients asking for the master at once.
t=$(now_ms)
for _ in $(seq 1 8); do curl -s -o /dev/null -w '%{http_code}\n' "$M/master.m3u8?token=$ST" & done > "$R/par"; wait
bad=$(grep -vc '^200$' "$R/par")
[ "$bad" = "0" ] && report PASS parallel8 "$t" "8x200" || report FAIL parallel8 "$t" "non-200=$bad"

# Forced transcode: the first segment comes out of ffmpeg, served by the segment cache.
t=$(now_ms)
read -r code tries <<<"$(first_segment "$M" "$ST" "&maxHeight=240&videoCodecs=h264" "$R")"
served=$(curl -s -D - -o /dev/null "$M/$(grep -v '^#' "$R/variant.m3u8" | head -1)" | tr -d '\r' | sed -n 's/^X-Evey-Served-By: //p')
[ "$code" = "200" ] && [ "$served" = "transcode" ] && report PASS transcode "$t" "tries=$tries served-by=$served" || report FAIL transcode "$t" "http=$code served-by=${served:-none}"

# Cache disk full: pull every transcoded segment through the 1 MiB cache. Expected:
# the server stays up and a full cache yields ONE refusal, not a fresh ffmpeg per
# request. Known defect (2026-10-08): ffmpeg is restarted on every request while the
# cache is full, 503 forever. XFAIL until fixed.
t=$(now_ms); codes=""; reqs=0
starts0=$(grep -c '"hls: ffmpeg start"' <(docker exec "$NAME" sh -c 'cat /data/logs/*.log' 2>/dev/null) || true)
for s in $(grep -v '^#' "$R/variant.m3u8"); do
    c=503; k=0
    while [ "$c" = "503" ] && [ $k -lt 10 ]; do c=$(curl -s -o /dev/null -w '%{http_code}' "$M/$s"); k=$((k+1)); [ "$c" = "503" ] && sleep 0.2; done
    codes="$codes$c "; reqs=$((reqs+1))
done
starts1=$(grep -c '"hls: ffmpeg start"' <(docker exec "$NAME" sh -c 'cat /data/logs/*.log' 2>/dev/null) || true)
restarts=$(( ${starts1:-0} - ${starts0:-0} ))
alive=$(curl -sf -o /dev/null "$B/healthz" && echo yes || echo no)
usage=$(docker exec "$NAME" sh -c "df -k /cache | tail -1 | awk '{print \$5}'")
if [ "$alive" != "yes" ]; then report FAIL disk_full "$t" "server down; codes=[$codes]"
elif [ "$restarts" -le 3 ]; then report XPASS disk_full "$t" "ffmpeg starts=$restarts cache=$usage - remove the XFAIL mark"
else report XFAIL disk_full "$t" "ffmpeg starts=$restarts for $reqs requests, cache=$usage, codes=[$codes]"; fi

# Library root unreadable: a scan must not prune what it cannot see.
t=$(now_ms)
chmod 000 "$R/media"; scan "$B" "$TOK" "$LIB"; read -r N2 _ <<<"$(items "$B" "$TOK")"; chmod 755 "$R/media"
[ "${N2:-0}" = "1" ] && report PASS unreachable "$t" "items kept=$N2" || report FAIL unreachable "$t" "items=${N2:-?}"

# SIGKILL during a playback, boot 2 on the same /data: login and catalogue survive.
t=$(now_ms)
curl -s -o /dev/null "$M/master.m3u8?token=$ST&maxHeight=240&videoCodecs=h264" &
docker kill -s KILL "$NAME" >/dev/null; docker start "$NAME" >/dev/null
wait_healthy "$B"
TOK=$(login "$B")
read -r N3 _ <<<"$(items "$B" "$TOK")"
rec=$(docker logs "$NAME" 2>&1 | grep -c 'automatic recovery in progress')
[ -n "$TOK" ] && [ "${N3:-0}" = "1" ] && report PASS kill9 "$t" "login ok, items=$N3, pg recovery=$rec" || report FAIL kill9 "$t" "login=${TOK:+ok}${TOK:-failed} items=${N3:-?}"

echo "total $(fmt "$(since "$START")") s  image=$IMG commit=$COMMIT  failed=$FAILED  logs=$OUT" | tee -a "$OUT/run.log"
exit $FAILED
