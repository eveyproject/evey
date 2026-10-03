#!/usr/bin/env bash
# Shared plumbing for the `make tv-*` family.
#
# Why this exists: bringing the TV app up used to be a dozen manual steps whose
# failure modes all looked identical from the outside - a screen that does not
# change. In one session the following each cost a detour: adb not attached,
# the emulator not authorised, the AVD exposing no D-pad, a Metro on a
# non-default port reached through a reverse tunnel nobody had written down,
# and - worst - a STALE BUNDLE, which produced a wrong diagnosis because the
# device was faithfully running week-old code.
#
# Everything below is idempotent on purpose: running it twice must be safe, so
# `tv-up` can be the one command you type without thinking about state.
#
# iOS note: only the adb-driven functions are Android-specific. The app-side
# contract they rely on (the EVEY| log prefix, the boot marker) is shared with
# tvOS, so a future tv-up-ios reimplements the transport, not the protocol.

set -euo pipefail

TV_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TV_APP_DIR="$TV_ROOT/frontend/apps/tv"

# Overridable from the environment so a machine with a different SDK layout, a
# second AVD or a busy port needs no edit to this file.
: "${ANDROID_SDK:=$HOME/Applications/Android/Sdk}"
: "${ADB:=$ANDROID_SDK/platform-tools/adb}"
: "${EMULATOR:=$ANDROID_SDK/emulator/emulator}"
: "${TV_AVD:=tv}"
# The emulator serves adb on 5555 but the adb server does not always discover
# it, hence the explicit connect below.
: "${TV_SERIAL:=127.0.0.1:5555}"
: "${TV_METRO_PORT:=8082}"
# The dev build asks for its bundle on 8081 (Metro's default, baked at build
# time); the reverse tunnel is what points that at our own port.
: "${TV_DEVICE_METRO_PORT:=8081}"
: "${TV_API_PORT:=9091}"
: "${TV_PKG:=com.evey.app}"
: "${TV_ACTIVITY:=.MainActivity}"

# Credentials for tv-login are NEVER stored in the repo. Put them in
# scripts/tv/.env (gitignored) or export them before calling.
if [[ -f "$TV_ROOT/scripts/tv/.env" ]]; then
  # shellcheck disable=SC1091
  . "$TV_ROOT/scripts/tv/.env"
fi

# --- output ------------------------------------------------------------------
# Colour only when attached to a terminal, so piping to a file or to a tool
# yields plain text.
if [[ -t 1 ]]; then
  C_OK=$'\033[32m'; C_WARN=$'\033[33m'; C_ERR=$'\033[31m'; C_DIM=$'\033[2m'; C_OFF=$'\033[0m'
else
  C_OK=""; C_WARN=""; C_ERR=""; C_DIM=""; C_OFF=""
fi

say()  { printf '%s\n' "$*"; }
ok()   { printf '%s✓%s %s\n' "$C_OK" "$C_OFF" "$*"; }
warn() { printf '%s!%s %s\n' "$C_WARN" "$C_OFF" "$*"; }
fail() { printf '%s✗%s %s\n' "$C_ERR" "$C_OFF" "$*" >&2; }
step() { printf '%s→%s %s\n' "$C_DIM" "$C_OFF" "$*"; }

# die prints the remedy, not just the symptom: every failure here has one
# obvious next command, and making the script say it is the whole point.
die() { fail "$1"; [[ $# -gt 1 ]] && printf '  %s\n' "$2" >&2; exit 1; }

# --- adb ---------------------------------------------------------------------
adb_() { "$ADB" -s "$TV_SERIAL" "$@"; }

emulator_running() { pgrep -f "qemu-system-x86_64 -avd $TV_AVD" >/dev/null 2>&1; }

device_ready() {
  [[ "$("$ADB" -s "$TV_SERIAL" get-state 2>/dev/null || true)" == "device" ]]
}

# Attach the running emulator. The adb server frequently does not auto-discover
# an emulator it did not start, and `adb connect` on an unauthorised device
# reports success while leaving it unusable - so the state is checked, not the
# exit code.
tv_connect() {
  local deadline=$((SECONDS + ${1:-60}))
  while (( SECONDS < deadline )); do
    device_ready && return 0
    "$ADB" connect "$TV_SERIAL" >/dev/null 2>&1 || true
    if [[ "$("$ADB" devices | grep -c unauthorized || true)" != "0" ]]; then
      die "l'émulateur refuse adb (unauthorized)" \
          "accepte la popup « Autoriser le débogage » sur l'écran de l'émulateur, puis relance."
    fi
    sleep 1
  done
  return 1
}

boot_completed() {
  [[ "$(adb_ shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" == "1" ]]
}

# Reverse tunnels are bound to the adb TRANSPORT, not to the device: any
# reconnection silently leaves stale entries in `adb reverse --list` that no
# longer forward anything. Re-adding them is cheap and idempotent, so it is
# done on every bring-up rather than only when the list looks empty.
tv_reverse() {
  adb_ reverse --remove-all >/dev/null 2>&1 || true
  adb_ reverse "tcp:$TV_DEVICE_METRO_PORT" "tcp:$TV_METRO_PORT" >/dev/null
  adb_ reverse "tcp:$TV_API_PORT" "tcp:$TV_API_PORT" >/dev/null
}

# Metro is started with `setsid` in up.sh and metro-restart.sh, not a plain
# `&`: a background job stays in the process group of whoever ran the command,
# so closing the terminal - or any tool that kills the group on timeout - took
# the bundler down with it, and the next command then reported a perfectly
# healthy setup as broken.

# --- metro -------------------------------------------------------------------
metro_up() { curl -s -o /dev/null --max-time 2 "http://localhost:$TV_METRO_PORT/status"; }

# The exact URL the dev build requests. Fetching it once forces Metro to finish
# transforming BEFORE the app connects: an app that attaches mid-build has been
# observed dying with a SIGSEGV inside libreactnative, which reads as "the app
# crashes" and sends you hunting a bug in your own code.
tv_bundle_url() {
  printf 'http://localhost:%s/node_modules/expo-router/entry.bundle?platform=android&dev=true&minify=false&app=%s&transform.routerRoot=app' \
    "$TV_METRO_PORT" "$TV_PKG"
}

app_running() { adb_ shell pidof "$TV_PKG" >/dev/null 2>&1; }

current_activity() {
  adb_ shell dumpsys activity activities 2>/dev/null \
    | grep -oE 'topResumedActivity=[^ ]+ [^ ]+ [^ ]+' | head -1 | awk '{print $NF}'
}

# The commit the working tree is on. Injected into Metro's environment so the
# app can announce which code it is actually running - the single fact that
# would have saved the longest wrong turn of the session that motivated these
# scripts.
tv_build_sha() { git -C "$TV_ROOT" rev-parse --short HEAD 2>/dev/null || echo unknown; }

# The sha Metro was STARTED with, recorded because that is what the app's boot
# marker can possibly say.
#
# EXPO_PUBLIC_* values are inlined when a module is transformed, so the marker
# is fixed at Metro's launch and does NOT follow later commits. Comparing it to
# HEAD therefore cries "stale bundle" on a perfectly fresh app the moment you
# commit anything - Metro recompiles changed files on the fly, so the CODE is
# current even when the label is old. The question worth asking is narrower and
# this file is what makes it answerable: did the app reload since Metro
# started?
TV_METRO_SHA_FILE="${TMPDIR:-/tmp}/evey-tv-metro.sha"
metro_started_sha() { cat "$TV_METRO_SHA_FILE" 2>/dev/null || echo unknown; }
