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
#   G. mixed-owner media mounts -> no adoption, fall back to 1000
#
# Usage: scripts/entrypoint-sim-test.sh   (needs docker)
set -eu

cd "$(dirname "$0")/.."
IMG=debian:trixie-slim
ENTRY="$PWD/docker/entrypoint.sh"
RENDER_GID=105

# MEDIA_SPEC (optional): space-separated "<dir>=<uid>:<gid>" entries creating
# fake media mounts before the entrypoint runs, e.g. "/media=4242:4242".
setup='
    groupadd -g 1000 evey
    useradd -u 1000 -g evey -m -s /usr/sbin/nologin evey
    useradd -u 1001 -g evey -m -s /usr/sbin/nologin evey-plugin
    mkdir -p /data/plugins /cache
    chown -R evey:evey /data /cache
    chown evey-plugin:evey /data/plugins
    for spec in ${MEDIA_SPEC:-}; do
        dir="${spec%%=*}"; own="${spec#*=}"
        mkdir -p "$dir"
        chown "$own" "$dir"
    done
    printf "#!/bin/sh\ngrep -E \"^(Uid|Gid|Groups):\" /proc/self/status\n" > /evey
    chmod +x /evey
    exec /entrypoint.sh
'

run_case() { # $@ = extra docker run args
    docker run --rm "$@" \
        -v "$ENTRY:/entrypoint.sh:ro" \
        -e EVEY_EMBEDDED_MEILI=false \
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

echo "--- case A: default PUID/PGID + group_add $RENDER_GID"
outA=$(run_case --group-add "$RENDER_GID")
echo "$outA"
assert_uid "$outA" 1000 A
assert_group "$outA" "$RENDER_GID" A
refute_group "$outA" 0 A

echo "--- case B: PUID/PGID remap 1500 + group_add $RENDER_GID"
outB=$(run_case --group-add "$RENDER_GID" -e PUID=1500 -e PGID=1500)
echo "$outB"
assert_uid "$outB" 1500 B
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

echo "--- case G: mixed-owner media -> no adoption, fallback 1000"
outG=$(run_case -e MEDIA_SPEC="/media=4242:4242 /media2=5555:5555")
echo "$outG"
assert_uid "$outG" 1000 G

if [ "$fail" -ne 0 ]; then
    echo "entrypoint sim-test: FAILED"
    exit 1
fi
echo "entrypoint sim-test: all cases green"
