#!/usr/bin/env bash
# tv-logs - the app's own log, and nothing else.
#
# `adb logcat` is a firehose: finding the one line that mattered ("[player]
# native playback error ... 404") meant grepping blind through Bluetooth stats
# and Glide chatter. This narrows to the app's PID, keeps the JS console and
# native crashes, drops the system noise, and colours by level.
#
# Usage: tv-logs [--follow] [--since 5m] [--all] [pattern]
#   --all   keep system lines too (for when the app dies before it logs)
set -uo pipefail
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

FOLLOW=false; SINCE=""; ALL=false; PATTERN=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --follow|-f) FOLLOW=true; shift ;;
    --since) SINCE="$2"; shift 2 ;;
    --all) ALL=true; shift ;;
    *) PATTERN="$1"; shift ;;
  esac
done

device_ready || die "aucun device attaché" "make tv-up"

# -d dumps and exits; without it logcat streams.
ARGS=(-v time)
[[ "$FOLLOW" == false ]] && ARGS+=(-d)
[[ -n "$SINCE" ]] && ARGS+=(-t "$SINCE")

# The app's JS goes to the ReactNativeJS tag; native crashes to DEBUG/libc; our
# own structured lines carry the EVEY| prefix (see src/lib/log.ts).
KEEP='ReactNativeJS|EVEY\||AndroidRuntime|libc *:|DEBUG *:|ExoPlayer|MediaCodec|ExpoModulesCore'
DROP='Glide|Choreographer|BluetoothPowerStats|ConfigUpdater|cast_shell|TinyalsaSink'

# `|| true` on the filters: no match is an ordinary outcome (a quiet app), and
# letting grep's exit code escape made `make tv-logs` report a failure.
adb_ logcat "${ARGS[@]}" 2>/dev/null \
  | { grep -E "$KEEP" || true; } \
  | { grep -vE "$DROP" || true; } \
  | { if [[ -n "$PATTERN" ]]; then grep -iE "$PATTERN" || true; else cat; fi; } \
  | sed -E \
      -e "s/(EVEY\|error\|[^$]*)/$(printf '\033[31m')\1$(printf '\033[0m')/" \
      -e "s/(EVEY\|warn\|[^$]*)/$(printf '\033[33m')\1$(printf '\033[0m')/" \
      -e "s/(FATAL|AndroidRuntime|SIGSEGV)/$(printf '\033[31m')\1$(printf '\033[0m')/"
