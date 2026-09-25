#!/usr/bin/env bash
# Media3/ExoPlayer smoke test of Evey's manifest.mpd on a booted emulator or
# device (adb): builds the app (gradle), installs it, reverses the dev
# server's port, and runs the scenario (start, seek, rung and audio switches,
# end of stream, then a resume at 45 s) for each media, reading the verdicts
# off logcat (tag EveySmoke). Muted. Exit code 1 when a step fails.
#
#   EVEY_USER=... EVEY_PASS=... scripts/dash-harness/android/smoke.sh <mediaId>[:<query>] ...
#
# ANDROID_HOME (default ~/Applications/Android/Sdk), JAVA_HOME (JDK 17),
# GRADLE (a gradle 8.13 binary), EVEY_URL (default http://localhost:9091).
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
ANDROID_HOME=${ANDROID_HOME:-$HOME/Applications/Android/Sdk}
adb=$ANDROID_HOME/platform-tools/adb
base=${EVEY_URL:-http://localhost:9091}
port=${base##*:}

if [ -n "${GRADLE:-}" ]; then
  (cd "$here" && echo "sdk.dir=$ANDROID_HOME" > local.properties && "$GRADLE" -q assembleDebug)
fi
$adb install -r "$here/app/build/outputs/apk/debug/app-debug.apk" >/dev/null
$adb reverse tcp:"$port" tcp:"$port" >/dev/null

token=$(curl -s -X POST "$base/api/v1/auth/login" -H 'Content-Type: application/json' \
  -d "{\"username\":\"$EVEY_USER\",\"password\":\"$EVEY_PASS\"}" | python3 -c 'import json,sys; print(json.load(sys.stdin)["accessToken"])')

failed=0
run() { # url extras...
  $adb logcat -c
  $adb shell "am start -S -W -n com.evey.dashsmoke/.MainActivity --es url '$1' ${2:-}" >/dev/null
  local end=$((SECONDS + 400)) log=""
  while [ $SECONDS -lt $end ]; do
    log=$($adb logcat -d -s EveySmoke:I | tr -d '\r' | grep -o 'SMOKE .*' || true)
    echo "$log" | grep -q 'SMOKE DONE' && break
    sleep 2
  done
  echo "$log" | sed 's/^SMOKE //'
  echo "$log" | grep -q 'SMOKE DONE' || echo "FAIL no verdict in 400 s"
}
for target in "$@"; do
  media=${target%%:*}
  extra=""; [ "$target" != "$media" ] && extra="&${target#*:}"
  url="$base/api/v1/media/$media/hls/manifest.mpd?videoCodecs=h264&audioCodecs=aac&containers=mp4&token=$token$extra"
  echo "== $media"
  out=$(run "$url"; run "$url" "--el startMs 45000 --ez resumeOnly true")
  echo "$out" | sed 's/^/   /'
  echo "$out" | grep -q "^FAIL" && failed=1
done
exit $failed
