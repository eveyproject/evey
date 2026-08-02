#!/usr/bin/env bash
# tv-reset - wipe the app's on-device state and come back up clean.
#
# Earned the hard way: restarting Metro with --clear underneath a running app
# left the device with a corrupted dev-bundle cache, and the app then died with
# a SIGSEGV inside libreactnative about 100 ms after boot - every single
# launch. No JavaScript guard can catch that, and nothing in the app's own logs
# explains it: the only tell is that it dies right after the boot marker.
#
# `pm clear` drops the cache (and the session, which tv-login restores).
set -euo pipefail
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

device_ready || die "aucun device attaché" "make tv-up"
step "purge des données de $TV_PKG"
adb_ shell pm clear "$TV_PKG" >/dev/null
ok "données purgées (session comprise)"
exec "$(dirname "${BASH_SOURCE[0]}")/up.sh"
