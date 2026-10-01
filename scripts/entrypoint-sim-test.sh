#!/bin/sh
# Sim-test for docker/entrypoint.sh privilege handling, run against the same
# Debian base as the runtime image. Each case boots a throwaway container that
# recreates the image's users/dirs, substitutes /evey with a stub printing the
# process credentials, then runs the real entrypoint. Verifies:
#
#   A. runtime-granted supplementary groups (compose `group_add:`, e.g. the
#      host render group for /dev/dri) survive the setpriv privilege drop
#   B. same with a PUID/PGID remap
#   C. no group_add -> only the evey group; root's group 0 never leaks
#   D. no PUID/PGID env -> the media mounts' owner is auto-adopted
#   E. explicit PUID/PGID beats media-owner auto-detection
#   F. root-owned media -> no adoption, fall back to 1000
#   G. mixed-owner media -> majority owner adopted, other mounts' groups joined
#   H. root + regular mixed -> the regular owner is adopted (root never is)
#   I. /data pre-created by the operator as PUID (Synology) while the cache
#      volume is still the image's 1000 -> /cache is re-owned on its own test
#   P. embedded Postgres (needs an evey image, EVEY_PG_IMG): the unix socket
#      refuses a passwordless login, a legacy trust pg_hba.conf is migrated,
#      a rotated EVEY_DB_PASSWORD is realigned
#
# Usage: scripts/entrypoint-sim-test.sh   (needs docker)
#        EVEY_PG_IMG=gravityz/evey:beta-0.0.1 scripts/entrypoint-sim-test.sh
set -eu

cd "$(dirname "$0")/.."
IMG=debian:trixie-slim
ENTRY="$PWD/docker/entrypoint.sh"
RENDER_GID=105

# MEDIA_SPEC (optional): space-separated "<dir>=<uid>:<gid>" entries creating
# fake media mounts before the entrypoint runs, e.g. "/media=4242:4242".
# DATA_OWNER (optional): "<uid>:<gid>" the operator pre-created ./evey-data
# with (a bind mount hides the image's /data, so plugins/ is absent too).
setup='
    groupadd -g 1000 evey
    useradd -u 1000 -g evey -m -s /usr/sbin/nologin evey
    useradd -u 1001 -g evey -m -s /usr/sbin/nologin evey-plugin
    mkdir -p /data/plugins /cache
    chown -R evey:evey /data /cache
    chmod 750 /data/plugins
    if [ -n "${DATA_OWNER:-}" ]; then
        rm -rf /data/plugins
        chown "$DATA_OWNER" /data
    fi
    for spec in ${MEDIA_SPEC:-}; do
        dir="${spec%%=*}"; own="${spec#*=}"
        mkdir -p "$dir"
        chown "$own" "$dir"
    done
    printf "#!/bin/sh\ngrep -E \"^(Uid|Gid|Groups):\" /proc/self/status\necho \"plugins=\$(stat -c \"%%u:%%g %%a\" /data/plugins)\"\necho \"cache=\$(stat -c \"%%u:%%g\" /cache)\"\n" > /evey
    chmod +x /evey
    exec /entrypoint.sh
'

run_case() { # $@ = extra docker run args
    docker run --rm "$@" \
        -v "$ENTRY:/entrypoint.sh:ro" \
        -e EVEY_EMBEDDED_MEILI=false \
        -e EVEY_EMBEDDED_POSTGRES=false \
        "$IMG" sh -ec "$setup"
}

fail=0
assert_uid() { # $1=output $2=uid $3=case
    echo "$1" | grep -Eq "^Uid:[[:space:]]+$2[[:space:]]" \
        || { echo "FAIL [$3]: expected uid $2"; fail=1; }
}
assert_group() { # $1=output $2=gid $3=case
    echo "$1" | grep '^Groups:' | grep -qw "$2" \
        || { echo "FAIL [$3]: supplementary group $2 missing (lost across privilege drop)"; fail=1; }
}
refute_group() { # $1=output $2=gid $3=case
    echo "$1" | grep '^Groups:' | grep -qw "$2" \
        && { echo "FAIL [$3]: supplementary group $2 must not be present"; fail=1; } || true
}
# The SERVER must own /data/plugins (it extracts installs there); the plugin
# uid (1001) only reads via the group. Prod 2026-07-11: the old restore gave
# the dir to evey-plugin, breaking every plugin install with EACCES.
assert_plugins_dir() { # $1=output $2=uid:gid $3=case
    echo "$1" | grep -q "^plugins=$2 750$" \
        || { echo "FAIL [$3]: /data/plugins must be owned $2 mode 750 (got: $(echo "$1" | grep '^plugins='))"; fail=1; }
}

echo "--- case A: default PUID/PGID + group_add $RENDER_GID"
outA=$(run_case --group-add "$RENDER_GID")
echo "$outA"
assert_uid "$outA" 1000 A
assert_plugins_dir "$outA" "1000:1000" A
assert_group "$outA" "$RENDER_GID" A
refute_group "$outA" 0 A

echo "--- case B: PUID/PGID remap 1500 + group_add $RENDER_GID"
outB=$(run_case --group-add "$RENDER_GID" -e PUID=1500 -e PGID=1500)
echo "$outB"
assert_uid "$outB" 1500 B
assert_plugins_dir "$outB" "1500:1500" B
assert_group "$outB" "$RENDER_GID" B
refute_group "$outB" 0 B

echo "--- case C: no group_add -> evey group only, no root leak"
outC=$(run_case)
echo "$outC"
assert_uid "$outC" 1000 C
assert_group "$outC" 1000 C
refute_group "$outC" 0 C
refute_group "$outC" "$RENDER_GID" C

echo "--- case D: no PUID/PGID env -> media owner auto-adopted"
outD=$(run_case --group-add "$RENDER_GID" -e MEDIA_SPEC="/media=4242:4242 /media2=4242:4242")
echo "$outD"
assert_uid "$outD" 4242 D
assert_plugins_dir "$outD" "4242:4242" D
assert_group "$outD" "$RENDER_GID" D
refute_group "$outD" 0 D

echo "--- case E: explicit PUID/PGID beats auto-detection"
outE=$(run_case -e PUID=1500 -e PGID=1500 -e MEDIA_SPEC="/media=4242:4242")
echo "$outE"
assert_uid "$outE" 1500 E

echo "--- case F: root-owned media -> no adoption, fallback 1000"
outF=$(run_case -e MEDIA_SPEC="/media=0:0")
echo "$outF"
assert_uid "$outF" 1000 F

echo "--- case G: mixed owners -> majority adopted, other mounts' groups joined"
outG=$(run_case -e MEDIA_SPEC="/media=4242:4242 /media2=4242:4242 /media3=5555:5556")
echo "$outG"
assert_uid "$outG" 4242 G
assert_group "$outG" 5556 G
refute_group "$outG" 0 G

echo "--- case H: root + regular mixed -> the regular owner is adopted"
outH=$(run_case -e MEDIA_SPEC="/media=0:0 /media2=4242:4242")
echo "$outH"
assert_uid "$outH" 4242 H
refute_group "$outH" 0 H

echo "--- case I: /data pre-owned by PUID, /cache still 1000 -> /cache re-owned, plugins/ created"
outI=$(run_case -e PUID=1026 -e PGID=100 -e DATA_OWNER=1026:100)
echo "$outI"
assert_uid "$outI" 1026 I
assert_plugins_dir "$outI" "1026:100" I
echo "$outI" | grep -q "^cache=1026:100$" \
    || { echo "FAIL [I]: /cache must be re-owned 1026:100 even when /data already is (got: $(echo "$outI" | grep '^cache='))"; fail=1; }

# Embedded Postgres cases run the real image with /evey swapped for a stub that
# probes the socket as the server uid, then exits (no server, no Meili).
pg_stub='
    printf "#!/bin/sh\nS=/tmp/evey-pg\npsql -w -h \$S -U evey -d evey -tAc \"select 1\" >/dev/null 2>&1 && echo nopw=ok || echo nopw=refused\nPGPASSWORD=\"\${EVEY_DB_PASSWORD:-\$(cat /data/keys/postgres.password)}\" psql -h \$S -U evey -d evey -tAc \"select 1\" >/dev/null 2>&1 && echo pw=ok || echo pw=refused\n" > /evey
    chmod +x /evey
    exec /entrypoint.sh
'
pg_case() { # $1=volume, rest = extra docker run args
    vol="$1"; shift
    docker run --rm "$@" -v "$ENTRY:/entrypoint.sh:ro" -v "$vol:/data" \
        -e EVEY_EMBEDDED_MEILI=false --entrypoint sh "$EVEY_PG_IMG" -ec "$pg_stub" 2>&1
}
assert_line() { # $1=output $2=line $3=case
    echo "$1" | grep -qx "$2" || { echo "FAIL [$3]: expected '$2'"; fail=1; }
}
if [ -n "${EVEY_PG_IMG:-}" ]; then
    vol="evey-sim-pg-$$"
    docker volume create "$vol" >/dev/null
    echo "--- case P1: fresh cluster -> socket asks the password"
    out=$(pg_case "$vol"); echo "$out" | grep -E "^(evey|nopw|pw)"
    assert_line "$out" nopw=refused P1; assert_line "$out" pw=ok P1
    echo "--- case P2: legacy trust pg_hba.conf -> migrated to scram"
    docker run --rm -u 1000:1000 -v "$vol:/data" --entrypoint sh "$EVEY_PG_IMG" -c \
        "sed -i -E 's/^(local[[:space:]].*)scram-sha-256\$/\\1trust/' /data/postgres/pg_hba.conf"
    out=$(pg_case "$vol"); echo "$out" | grep -E "^(evey|nopw|pw)"
    echo "$out" | grep -q "initdb's local trust lines moved to scram-sha-256" || { echo "FAIL [P2]: migration not reported"; fail=1; }
    assert_line "$out" nopw=refused P2; assert_line "$out" pw=ok P2
    echo "--- case P3: rotated EVEY_DB_PASSWORD -> realigned"
    out=$(pg_case "$vol" -e EVEY_DB_PASSWORD=sim-rotated); echo "$out" | grep -E "^(evey|nopw|pw)"
    echo "$out" | grep -q "realigning the role password" || { echo "FAIL [P3]: no realignment"; fail=1; }
    assert_line "$out" nopw=refused P3; assert_line "$out" pw=ok P3
    docker volume rm "$vol" >/dev/null
else
    echo "--- cases P*: skipped (set EVEY_PG_IMG to an evey image to run them)"
fi

if [ "$fail" -ne 0 ]; then
    echo "entrypoint sim-test: FAILED"
    exit 1
fi
echo "entrypoint sim-test: all cases green"
