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
#      (tests/ops/test-results) · OPS_NAME container name (evey-ops-PORT; not
#      NAME, which shells set to the host name) · KEEP=1 leaves the container running.
#
# Exit: 0 all green, 1 a scenario failed. An XFAIL (known defect, referenced
# inline) does not fail the run; an XPASS does, so the mark gets removed.
#
set -u

IMG="${IMG:-eveyproject/evey:ops-local}"
PORT="${PORT:-19092}"
NAME="${OPS_NAME:-evey-ops-$PORT}"
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

# The clip comes from the image's own ffmpeg: 120 s, two audio tracks, owned by
# the caller. Its mtime is pushed back so the scan does not wait for it to settle.
t=$(now_ms)
make_clip "$R/media" "$IMG" 2>"$OUT/ffmpeg.err" \
    && report PASS clip "$t" "$(stat -c %s "$R/media/$CLIP") bytes" \
    || { report FAIL clip "$t" "$(head -c 200 "$OUT/ffmpeg.err")"; exit 1; }

# Boot 1: empty /data, /cache on a 1 MiB tmpfs so the disk-full scenario can fill it.
t=$(now_ms)
docker run -d --name "$NAME" -p "127.0.0.1:$PORT:9091" -v "$R/data:/data" -v "$R/media:/media" \
    --tmpfs "/cache:size=16m,uid=$(id -u),gid=$(id -g)" --stop-timeout 60 -e EVEY_TITLE_PROBE=off -e EVEY_TRANSCODE_CACHE_MIN_FREE_MB=8 "$IMG" >/dev/null
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

# Cache disk full: the guard must keep playback alive. A segment already on disk
# still serves; a MISSING transcode segment is shed cleanly (503 cache_full,
# Retry-After) with NO ffmpeg respawn - before 2026-10-09 this restarted ffmpeg
# on every request (the storm). After space is freed, production resumes.
# The missing segment is the last one: the first session reads the clip at
# 1.5x real time past its 60 s burst, so it cannot have reached the end of a
# 120 s clip yet, whatever the CPU (2026-10-19: a 30 s clip fit in the burst,
# was encoded in half a second, and the verdict hung on a few ms). The last
# segment starts where the picture ends, so the recovery also proves a seek
# past the last frame is produced.
# Needs the server booted with a low free-space floor (EVEY_TRANSCODE_CACHE_MIN_FREE_MB).
t=$(now_ms)
curl -s -o "$R/m240.m3u8" "$M/master.m3u8?token=$ST&maxHeight=240&videoCodecs=h264"
PL=$(grep -v '^#' "$R/m240.m3u8" | grep m3u8 | head -1); curl -s -o "$R/pl240.m3u8" "$M/$PL"
SEG0=$(grep -v '^#' "$R/pl240.m3u8" | head -1); SEGL=$(grep -v '^#' "$R/pl240.m3u8" | tail -1)
# Produce seg0 so it is on disk, then fill the cache below the floor.
c=503; k=0; while [ "$c" = "503" ] && [ $k -lt 30 ]; do c=$(curl -s -o /dev/null -w '%{http_code}' "$M/$SEG0"); k=$((k+1)); [ "$c" = "503" ] && sleep 0.2; done
starts0=$(docker exec "$NAME" sh -c 'cat /data/logs/*.log' 2>/dev/null | grep -c '"hls: ffmpeg start"')
docker exec "$NAME" sh -c 'dd if=/dev/zero of=/cache/ops-filler bs=1M count=11 2>/dev/null || true'
cached=$(curl -s -o /dev/null -w '%{http_code}' "$M/$SEG0")                 # on disk: must still serve
missing=$(curl -s -D - -o /dev/null "$M/$SEGL" | tr -d '\r')               # missing: must shed cleanly
mcode=$(printf '%s' "$missing" | sed -n 's#^HTTP/[0-9.]* \([0-9]*\).*#\1#p' | head -1)
merr=$(printf '%s' "$missing" | sed -n 's/^[Xx]-[Ee]vey-[Ee]rror: //p' | tr -d '\n')
starts1=$(docker exec "$NAME" sh -c 'cat /data/logs/*.log' 2>/dev/null | grep -c '"hls: ffmpeg start"')
restarts=$(( ${starts1:-0} - ${starts0:-0} ))
alive=$(curl -sf -o /dev/null "$B/healthz" && echo yes || echo no)
# Free the space and confirm production resumes.
docker exec "$NAME" sh -c 'rm -f /cache/ops-filler'
c=503; k=0; while [ "$c" = "503" ] && [ $k -lt 30 ]; do c=$(curl -s -o /dev/null -w '%{http_code}' "$M/$SEGL"); k=$((k+1)); [ "$c" = "503" ] && sleep 0.2; done
detail="cached=$cached missing=$mcode/$merr ffmpeg=$restarts alive=$alive recover=$c"
if [ "$alive" = "yes" ] && [ "$cached" = "200" ] && [ "$mcode" = "503" ] && [ "$merr" = "cache_full" ] && [ "$restarts" -le 1 ] && [ "$c" = "200" ]; then
    report PASS disk_full "$t" "$detail"
else
    report FAIL disk_full "$t" "$detail"
fi

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
