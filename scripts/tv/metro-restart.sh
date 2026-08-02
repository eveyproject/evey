#!/usr/bin/env bash
# tv-metro-restart - restart Metro so the build marker is injected again.
#
# tv-up only injects EXPO_PUBLIC_BUILD_SHA when IT starts Metro. A bundler
# someone else started reports sha=unknown, which makes `tv-state` unable to
# answer the one question it exists for: is the device running my code?
#
# --clear is deliberately NOT passed: clearing the transform cache under a
# running app is what corrupted the device-side bundle in the first place. Use
# `make tv-reset` afterwards if you really need a cold cache.
set -euo pipefail
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

pkill -f "expo start --port $TV_METRO_PORT" 2>/dev/null || true
until ! metro_up; do sleep 1; done
step "redémarrage de Metro avec le marqueur de build $(tv_build_sha)"
( cd "$TV_APP_DIR" \
  && EXPO_TV=1 EXPO_PUBLIC_BUILD_SHA="$(tv_build_sha)" \
     npx expo start --port "$TV_METRO_PORT" \
     >"${TMPDIR:-/tmp}/evey-tv-metro.log" 2>&1 & )
until metro_up; do sleep 1; done
ok "Metro prêt"
say "→ make tv-up pour recharger l'app avec ce bundle"
