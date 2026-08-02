#!/usr/bin/env bash
# tv-up - bring the TV app up on the emulator, from any starting state.
#
# Target: under 30 s when the emulator is already running, well under a minute
# from cold. The two tricks that buy it:
#   - the emulator boots from its SNAPSHOT (no -no-snapshot), which is the
#     difference between ~10 s and ~90 s;
#   - Metro starts in parallel with the boot, and the bundle is warmed BEFORE
#     the app is launched, so the app never attaches to a half-built bundler.
#
# Idempotent: every step checks before acting, so this is the one command to
# type whatever state the machine is in.

set -euo pipefail
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

START=$SECONDS
NO_LOGIN=false
[[ "${1:-}" == "--no-login" ]] && NO_LOGIN=true

# 1. Metro first, in the background: it is the slowest thing to become useful
#    and it does not need the emulator.
if metro_up; then
  ok "Metro déjà en écoute sur $TV_METRO_PORT"
else
  step "démarrage de Metro (port $TV_METRO_PORT)"
  ( cd "$TV_APP_DIR" \
    && EXPO_TV=1 EXPO_PUBLIC_BUILD_SHA="$(tv_build_sha)" \
       npx expo start --port "$TV_METRO_PORT" \
       >"${TMPDIR:-/tmp}/evey-tv-metro.log" 2>&1 & )
fi

# 2. Emulator, also in parallel.
if emulator_running; then
  ok "émulateur « $TV_AVD » déjà lancé"
else
  step "démarrage de l'émulateur « $TV_AVD » (depuis snapshot)"
  # -no-audio only: dropping -no-snapshot is what makes this fast. swiftshader
  # keeps it working on a machine with no usable GPU passthrough.
  ( "$EMULATOR" -avd "$TV_AVD" -gpu swiftshader_indirect -no-audio \
      >"${TMPDIR:-/tmp}/evey-tv-emulator.log" 2>&1 & )
fi

# 3. Attach and wait for the guest to be usable.
step "attente de l'émulateur"
tv_connect 120 || die "impossible d'attacher $TV_SERIAL" "vérifie « $EMULATOR -list-avds » et l'écran de l'émulateur."
until boot_completed; do sleep 1; done
ok "émulateur prêt ($TV_SERIAL)"

# 4. Reverse tunnels. The device resolves "localhost" to ITSELF, so without
#    these the app reaches neither Metro nor the API - and the symptom is a
#    blank screen with no error anywhere.
tv_reverse
ok "tunnels: device:$TV_DEVICE_METRO_PORT → metro:$TV_METRO_PORT · device:$TV_API_PORT → api:$TV_API_PORT"

# 5. Wait for Metro, then warm the bundle. This is the step whose absence
#    produced a repeatable SIGSEGV inside libreactnative.
step "attente de Metro"
until metro_up; do sleep 1; done
step "construction du bundle (première fois : ~1 min)"
BUNDLE_BYTES=$(curl -s -o /dev/null -w '%{size_download}' --max-time 600 "$(tv_bundle_url)")
(( BUNDLE_BYTES > 100000 )) || die "Metro n'a pas produit de bundle exploitable ($BUNDLE_BYTES octets)" \
  "regarde ${TMPDIR:-/tmp}/evey-tv-metro.log"
ok "bundle prêt ($((BUNDLE_BYTES / 1024)) Kio, sha $(tv_build_sha))"

# 6. Launch, and make sure it STAYS up.
#
# Reconnecting to a Metro that has just restarted leaves the dev build with a
# bundle cache it cannot use: it boots, prints its marker, and dies ~100 ms
# later with a SIGSEGV inside libreactnative. No JavaScript guard can catch
# that, and from the outside it looks exactly like a bug in the app.
#
# The remedy is known (wipe the app's data) so this does it itself rather than
# leaving a trap for whoever types the command next.
launch() {
  adb_ shell am force-stop "$TV_PKG" >/dev/null 2>&1 || true
  adb_ shell am start -n "$TV_PKG/$TV_ACTIVITY" >/dev/null
  local i
  for i in $(seq 1 40); do app_running && break; sleep 1; done
  app_running || return 1
  # A live process is not a visible app: after a data wipe the activity has
  # been observed starting and immediately falling behind the TV launcher,
  # which left every later step dumping the launcher's view hierarchy and
  # reporting nonsense. Insist on the foreground.
  for i in $(seq 1 15); do
    [[ "$(current_activity)" == "$TV_PKG/$TV_ACTIVITY" ]] && break
    adb_ shell am start -n "$TV_PKG/$TV_ACTIVITY" >/dev/null 2>&1 || true
    sleep 1
  done
  [[ "$(current_activity)" == "$TV_PKG/$TV_ACTIVITY" ]] || return 3
  # Survival check: a crash-on-boot dies well inside this window.
  for i in $(seq 1 12); do sleep 1; app_running || return 2; done
  return 0
}

step "lancement de $TV_PKG"
if launch; then
  ok "app lancée"
else
  warn "l'app est morte au démarrage - cache de bundle périmé, purge et nouvelle tentative"
  adb_ shell pm clear "$TV_PKG" >/dev/null
  # pm clear drops the stored server URL too, so the login step below has to
  # re-enter it; login.sh handles that screen.
  if launch; then
    ok "app lancée après purge"
  else
    die "l'app ne tient pas au démarrage même après purge" \
        "make tv-logs --all pour la trace native"
  fi
fi

if [[ "$NO_LOGIN" == false ]]; then
  "$(dirname "${BASH_SOURCE[0]}")/login.sh" || warn "connexion automatique impossible (voir make tv-login)"
fi

say ""
ok "prêt en $((SECONDS - START)) s"
say "   make tv-state   état courant     make tv-logs   journal de l'app"
say "   make tv-shot    capture d'écran  make tv-key K  touche télécommande"
