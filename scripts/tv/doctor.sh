#!/usr/bin/env bash
# tv-doctor - check every precondition and name the remedy.
#
# Each of these has silently produced "the screen does not change" at least
# once. A symptom that generic is worth a checklist that answers WHICH one.
set -uo pipefail
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

PROBLEMS=0
check() { # check <label> <command...> ; on failure prints the remedy in $REMEDY
  local label="$1"; shift
  if "$@" >/dev/null 2>&1; then ok "$label"; else
    fail "$label"; [[ -n "${REMEDY:-}" ]] && printf '    %s\n' "$REMEDY"
    PROBLEMS=$((PROBLEMS + 1))
  fi
}

REMEDY="installe le SDK Android ou exporte ANDROID_SDK=..." check "adb présent ($ADB)" test -x "$ADB"
REMEDY="make tv-up" check "émulateur « $TV_AVD » lancé" emulator_running
REMEDY="make tv-up (ou accepte la popup de débogage sur l'écran)" check "device attaché ($TV_SERIAL)" device_ready

if device_ready; then
  REMEDY="l'émulateur démarre encore, patiente" check "guest démarré" boot_completed

  # No D-pad means the remote cannot be simulated at all, and the whole app is
  # undrivable from a keyboard. It is an AVD setting, not an app bug.
  if adb_ shell dumpsys input 2>/dev/null | grep -q "Sources:.*DPAD"; then
    ok "D-pad exposé par l'AVD"
  else
    fail "aucun périphérique D-pad"
    printf '    %s\n' "mets hw.dPad = yes ET hw.keyboard = yes dans ~/.android/avd/$TV_AVD.avd/config.ini, puis redémarre l'émulateur"
    PROBLEMS=$((PROBLEMS + 1))
  fi

  for spec in "$TV_DEVICE_METRO_PORT→$TV_METRO_PORT" "$TV_API_PORT→$TV_API_PORT"; do
    dev_port="${spec%%→*}"
    if adb_ reverse --list 2>/dev/null | grep -q "tcp:$dev_port"; then
      ok "tunnel reverse tcp:$dev_port"
    else
      fail "tunnel reverse tcp:$dev_port absent"
      printf '    %s\n' "make tv-up (sans lui l'app ne joint ni Metro ni l'API, écran blanc sans erreur)"
      PROBLEMS=$((PROBLEMS + 1))
    fi
  done

  REMEDY="make tv-up" check "app installée ($TV_PKG)" bash -c "adb -s '$TV_SERIAL' shell pm list packages | grep -q '$TV_PKG'"
fi

REMEDY="make tv-up" check "Metro en écoute ($TV_METRO_PORT)" metro_up

if metro_up; then
  # A bundle that still builds is indistinguishable, from the outside, from an
  # app that hangs on a black screen.
  size=$(curl -s -o /dev/null -w '%{size_download}' --max-time 20 "$(tv_bundle_url)" || echo 0)
  if [[ "$size" -gt 100000 ]]; then ok "bundle servi ($((size / 1024)) Kio)"
  else fail "bundle non servi (taille $size)"; printf '    %s\n' "make tv-up, puis ${TMPDIR:-/tmp}/evey-tv-metro.log"; PROBLEMS=$((PROBLEMS + 1)); fi
fi

if [[ -n "${EVEY_TV_USER:-}" ]]; then ok "compte dev configuré (${EVEY_TV_USER})"
else warn "EVEY_TV_USER absent - la connexion automatique sera ignorée (scripts/tv/.env)"; fi

say ""
if [[ "$PROBLEMS" -eq 0 ]]; then ok "tout est en place"; else fail "$PROBLEMS problème(s)"; exit 1; fi
