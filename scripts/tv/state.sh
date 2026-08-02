#!/usr/bin/env bash
# tv-state - one screenful answering "what is running, and is it my code?".
#
# The longest wrong turn of the session that motivated these scripts was
# believing the device ran the current commit when it ran an older bundle. The
# build sha below is the answer to that, in one second.
set -uo pipefail
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

printf 'émulateur : %s\n' "$(emulator_running && echo "lancé ($TV_AVD)" || echo "arrêté")"
printf 'device    : %s\n' "$(device_ready && echo "$TV_SERIAL prêt" || echo "non attaché")"
printf 'Metro     : %s\n' "$(metro_up && echo "port $TV_METRO_PORT" || echo "arrêté")"
printf 'sha local : %s\n' "$(tv_build_sha)"

if device_ready; then
  printf 'app       : %s\n' "$(app_running && echo "en cours (pid $(adb_ shell pidof "$TV_PKG" | tr -d '\r'))" || echo "arrêtée")"
  printf 'écran     : %s\n' "$(current_activity || echo inconnu)"
  # The app announces its build at boot (see src/lib/log.ts bootMarker).
  # lib.sh runs with -e, so a grep that matches nothing would abort this
  # informational script: every lookup below is explicitly tolerant.
  loaded=$(adb_ logcat -d 2>/dev/null | grep -oE 'EVEY\|info\|boot\| boot sha=[^ ]+' | tail -1 | sed -E 's/.*sha=//' || true)
  if [[ -z "$loaded" ]]; then
    warn "aucun marqueur de build (app pas encore démarrée, ou journal purgé)"
  elif [[ "$loaded" == "unknown" ]]; then
    warn "bundle chargé : sha inconnu - Metro n'a pas été démarré par tv-up (relance : make tv-metro-restart)"
  elif [[ "$loaded" == "$(tv_build_sha)" ]]; then
    ok "bundle chargé : $loaded (à jour)"
  else
    warn "bundle chargé : $loaded ≠ sha local $(tv_build_sha) - make tv-up"
  fi
  last_err=$( (adb_ logcat -d 2>/dev/null | grep -E 'EVEY\|error\||FATAL|SIGSEGV' | tail -1) || true)
  if [[ -n "$last_err" ]]; then printf 'dernière erreur : %s\n' "${last_err:0:160}"; fi
  # An informational command must never fail: `make tv-state` returning 1
  # because the app happens to be stopped reads as "the tool is broken".
  exit 0
fi
