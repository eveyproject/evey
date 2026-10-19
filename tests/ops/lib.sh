#!/usr/bin/env bash
# lib.sh - shared pieces of the operations harness (sourced by run.sh and upgrade.sh).

for bin in docker curl python3; do
    command -v "$bin" >/dev/null || { echo "missing: $bin" >&2; exit 2; }
done

ADMIN_USER="opsadmin"
ADMIN_PASS="Ops-harness-$(od -An -N4 -tx1 /dev/urandom | tr -d ' \n')"
CLIP="Ops Clip (2026).mkv"
FAILED=0

now_ms() { date +%s%3N; }
since() { echo "$(( $(now_ms) - $1 ))"; }
fmt() { python3 -c "print(f'{$1/1000:.1f}')"; }
json() { python3 -c "import json,sys; d=json.load(sys.stdin); print($1)" 2>/dev/null; }

# report STATUS NAME T0 DETAIL - one line per scenario, appended to $OUT/run.log.
report() {
    local ms; ms=$(since "$3"); [ "$ms" -lt 0 ] && ms=0  # a WSL clock step can make a step read negative
    printf '%-5s %-14s %6s s  %s\n' "$1" "$2" "$(fmt "$ms")" "$4" | tee -a "$OUT/run.log"
    case "$1" in FAIL|XPASS) FAILED=1 ;; esac
}

# make_clip DIR IMG - 120 s test clip with two audio tracks, made by the image's own
# ffmpeg, owned by the caller, mtime pushed back so a scan does not wait for it to settle.
make_clip() {
    docker run --rm --user "$(id -u):$(id -g)" -v "$1:/out" --entrypoint ffmpeg "$2" -loglevel error -y \
        -f lavfi -i testsrc2=size=640x360:rate=25 -f lavfi -i sine=frequency=440 -f lavfi -i sine=frequency=660 \
        -t 120 -map 0:v -map 1:a -map 2:a -c:v libx264 -preset ultrafast -pix_fmt yuv420p -c:a aac \
        -metadata:s:a:0 language=eng -metadata:s:a:1 language=fra "/out/$CLIP" \
    && touch -d '-1 hour' "$1/$CLIP"
}

# wait_healthy URL [SECONDS] - polls /healthz; returns 1 on timeout.
wait_healthy() {
    local i; for i in $(seq 1 $(( ${2:-120} * 10 ))); do curl -sf -o /dev/null "$1/healthz" && return 0; sleep 0.1; done; return 1
}
commit_of() { curl -s "$1/healthz" | json 'd["commit"]'; }

# first_admin URL - creates the admin, prints its access token.
first_admin() {
    curl -s -X POST "$1/api/v1/admin/setup" -H 'content-type: application/json' \
        -d "{\"admin_username\":\"$ADMIN_USER\",\"admin_password\":\"$ADMIN_PASS\",\"admin_email\":\"ops@example.invalid\",\"server_name\":\"ops\",\"server_language\":\"en\"}" \
        | json 'd["accessToken"]'
}
login() {
    curl -s -X POST "$1/api/v1/auth/login" -H 'content-type: application/json' \
        -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" | json 'd.get("accessToken") or d.get("access_token") or ""'
}
# create_library URL TOKEN - one movie library on /media, prints its id.
create_library() {
    curl -s -X POST "$1/api/v1/libraries" -H "authorization: Bearer $2" -H 'content-type: application/json' \
        -d '{"name":"Ops Movies","type":"LIBRARY_TYPE_MOVIES","paths":["/media"]}' | json 'd["id"]'
}
# scan URL TOKEN LIB - the call streams progress until the scan is over.
scan() { curl -s -N -X POST "$1/api/v1/libraries/$3/scan" -H "authorization: Bearer $2" -H 'content-type: application/json' -d '{}' >/dev/null; }
# items URL TOKEN - prints "<count> <first id or ->".
items() {
    curl -s "$1/api/v1/media?page_size=5" -H "authorization: Bearer $2" \
        | json 'str(d["pagination"]["totalItems"])+" "+(d["items"][0]["id"] if d["items"] else "-")'
}
stream_token() { curl -s -X POST "$1/api/v1/media/$3/stream-token" -H "authorization: Bearer $2" | json 'd["token"]'; }

# first_segment HLS_BASE TOKEN QUERY WORKDIR - master, variant, first segment (retried while
# "not yet ready"); prints "<http code> <tries>". Leaves master.m3u8 and variant.m3u8 in WORKDIR.
first_segment() {
    local m="$1" st="$2" q="$3" w="$4" pl seg code=503 k=0
    curl -s -o "$w/master.m3u8" "$m/master.m3u8?token=$st$q"
    pl=$(grep -v '^#' "$w/master.m3u8" | grep m3u8 | head -1); curl -s -o "$w/variant.m3u8" "$m/$pl"
    seg=$(grep -v '^#' "$w/variant.m3u8" | head -1)
    while [ "$code" = "503" ] && [ $k -lt 50 ]; do code=$(curl -s -o "$w/seg" -w '%{http_code}' "$m/$seg"); k=$((k+1)); [ "$code" = "503" ] && sleep 0.2; done
    echo "$code $k"
}
