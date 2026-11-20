#!/usr/bin/env bash
# Takes the documentation screenshots from a throwaway demo instance and says
# which of docs/images they change.
#
# Usage: scripts/docs-shots/run.sh <workdir> [name ...]
#   IMAGE  the Evey image to photograph (default ghcr.io/eveyproject/evey:master)
#   PORT   host port of the instance (default 9292)
#
# The instance runs the published image in demo mode with its own /data and
# /cache under <workdir>. Every video the screenshots show only as a poster is
# a stand-in (scripts/docs-shots/standins), so the bootstrap downloads a few
# hundred megabytes instead of the whole demo. Results: <workdir>/webp/*.webp
# and <workdir>/webp/changed.txt. Needs docker, go, node (npm ci in tests/e2e,
# a Playwright chromium) and python3 with Pillow.
set -euo pipefail

work=${1:?usage: run.sh <workdir> [name ...]}
shift
image=${IMAGE:-ghcr.io/eveyproject/evey:master}
port=${PORT:-9292}
name=evey-docshots
repo=$(cd "$(dirname "$0")/../.." && pwd)
mkdir -p "$work"
work=$(cd "$work" && pwd)
mkdir -p "$work/bin" "$work/data/demo-media" "$work/cache"

cleanup() { docker rm -f "$name" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "== stand-ins"
(cd "$repo" && CGO_ENABLED=0 go build -o "$work/bin/standins" ./scripts/docs-shots/standins)
docker pull -q "$image"
# The folders go back to the image's own user: the server still writes the
# real downloads next to the stand-ins.
docker run --rm --entrypoint sh -v "$work/bin:/tools:ro" -v "$work/data/demo-media:/m" "$image" \
  -c '/tools/standins -dir /m && chown -R evey:evey /m'

echo "== demo instance ($image)"
cleanup
docker run -d --name "$name" -p "$port:9091" -e EVEY_DEMO=1 -e EVEY_SERVER_BASE_URL="http://localhost:$port" \
  -v "$work/data:/data" -v "$work/cache:/cache" "$image" >/dev/null

serverlog() { docker exec "$name" sh -c 'cat /data/logs/evey-*.log 2>/dev/null' || true; }
alive() {
  [ -n "$(docker ps -q -f "name=^${name}$")" ] && return 0
  docker logs "$name" 2>&1 | tail -40
  echo "the demo instance stopped" >&2
  exit 1
}

# The bootstrap seeds the viewing history last, once every file is downloaded
# (the server logs to /data/logs, not to docker logs). A title the library had
# not indexed yet at that moment is seeded at the next start only.
for _ in $(seq 1 270); do
  serverlog | grep -q "demo: history seeded" && break
  alive
  sleep 10
done
if ! serverlog | grep -q "demo: history seeded"; then
  serverlog | grep -E "demo:|ERROR" | tail -40
  echo "the demo bootstrap did not finish in 45 minutes" >&2
  exit 1
fi

# So start once more: nothing left to download, the rest of the history is
# seeded. Ready when no demo line has come for a minute.
docker restart "$name" >/dev/null
sleep 30
last=-1
for _ in $(seq 1 60); do
  alive
  now=$(serverlog | grep -c "demo:" || true)
  [ "$now" = "$last" ] && break
  last=$now
  sleep 60
done
echo "demo ready: $(serverlog | grep -c "demo: history seeded") history lines"

echo "== screenshots"
rm -rf "$work/png" "$work/webp"
BASE="http://localhost:$port" node "$repo/scripts/docs-shots/shoot.mjs" "$work/png" "$@"
if python3 -c 'import PIL' 2>/dev/null; then
  python3 "$repo/scripts/docs-shots/compare.py" "$work/png" "$work/webp" "$repo/docs/images"
else
  # No Pillow here: the same script in a throwaway Python container.
  docker run --rm -v "$repo:/repo:ro" -v "$work:/work" python:3.12-slim sh -c \
    "pip install -q --disable-pip-version-check Pillow==11.3.0 && python /repo/scripts/docs-shots/compare.py /work/png /work/webp /repo/docs/images && chown -R $(id -u):$(id -g) /work/webp"
fi
