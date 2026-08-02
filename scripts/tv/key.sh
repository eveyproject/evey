#!/usr/bin/env bash
# tv-key - press remote buttons by name.
#
# Remembering that OK is keycode 23 and that the source has to be the D-pad is
# exactly the kind of detail that turns a five-second check into a detour.
#
# Usage: tv-key up|down|left|right|ok|back|home|play|menu [répétitions]
set -uo pipefail
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

declare -A KEYS=(
  [up]=19 [down]=20 [left]=21 [right]=22 [ok]=23 [select]=23
  [back]=4 [home]=3 [menu]=82 [play]=85 [pause]=85 [playpause]=85
)
# Case-insensitive: `tv-key DOWN` is what fingers actually type, and rejecting
# it only produced a puzzling "touche inconnue" for a key that plainly exists.
NAME="$(printf '%s' "${1:-}" | tr '[:upper:]' '[:lower:]')"; COUNT="${2:-1}"
[[ -n "${KEYS[$NAME]:-}" ]] || die "touche inconnue « $NAME »" "au choix : ${!KEYS[*]}"
device_ready || die "aucun device attaché" "make tv-up"

for _ in $(seq 1 "$COUNT"); do
  # The dpad source matters: a plain `input keyevent` is delivered as a
  # keyboard event and the TV navigation may ignore it.
  adb_ shell input dpad keyevent "${KEYS[$NAME]}" >/dev/null 2>&1 \
    || adb_ shell input keyevent "${KEYS[$NAME]}" >/dev/null
  sleep 0.3
done
ok "$NAME ×$COUNT"
