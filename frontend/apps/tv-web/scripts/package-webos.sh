#!/bin/sh
# Builds the TV web client and wraps the SAME build into a webOS .ipk: the
# dist that the server serves under /tv, plus LG's appinfo.json and icons.
# Needs the LG CLI (npm i -g @webos-tools/cli); ares-package is looked up on
# PATH first, then in npm's global bin, which nvm-style installs leave off
# the PATH.
set -eu
cd "$(dirname "$0")/.."

ARES="$(command -v ares-package 2>/dev/null || true)"
if [ -z "$ARES" ]; then
  ARES="$(npm prefix -g)/bin/ares-package"
fi
if [ ! -x "$ARES" ]; then
  echo "ares-package not found: npm i -g @webos-tools/cli" >&2
  exit 1
fi

pnpm run build
STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT
cp -R dist/. "$STAGE/"
cp webos/appinfo.json webos/icon.png webos/largeIcon.png "$STAGE/"
mkdir -p out
"$ARES" --outdir out "$STAGE"
echo "package: $(ls -1 out/*.ipk | tail -1)"
echo "install on a TV in developer mode: ares-install --device <name> out/<file>.ipk"
echo "or launch in the simulator: ares-launch --simulator <webOS TV version> $STAGE"
