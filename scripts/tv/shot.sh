#!/usr/bin/env bash
# tv-shot - screenshot to a known path, downscaled without extra tooling.
#
# The machine had neither PIL nor ImageMagick, so a full 1920x1080 PNG was the
# only option and it is heavy to move around. The emulator itself can scale,
# which needs nothing installed.
#
# Usage: tv-shot [nom]   → écrit dans /tmp/evey-tv/<nom>.png
set -uo pipefail
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

device_ready || die "aucun device attaché" "make tv-up"
OUT_DIR="${EVEY_TV_SHOTS:-${TMPDIR:-/tmp}/evey-tv}"
mkdir -p "$OUT_DIR"
NAME="${1:-shot-$(date +%H%M%S)}"
OUT="$OUT_DIR/$NAME.png"

adb_ exec-out screencap -p > "$OUT"
[[ -s "$OUT" ]] || die "capture vide" "l'écran est peut-être protégé (DRM)"
printf '%s\n' "$OUT"
