#!/usr/bin/env bash
# Builds the pre-packaged plugin zips under tests/plugins/ from the
# source trees in examples/plugins/. Run this after any example change
# or after a manifest-schema bump so the uploadable bundles stay in
# sync with the documented examples.
#
# For plugins that ship a package.json with a "build" script (e.g. a
# Vite/esbuild bundle for the front module), runs `npm install` and
# `npm run build` before zipping. node_modules/ is excluded from the
# zip — only dist/ + sources are shipped.
#
# Usage: ./scripts/build-test-plugins.sh

set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
src="$root/examples/plugins"
out="$root/tests/plugins"

mkdir -p "$out"

for dir in "$src"/*/; do
  [ -d "$dir" ] || continue
  [ -f "$dir/plugin.yaml" ] || { echo "skip $(basename "$dir"): no plugin.yaml"; continue; }

  name=$(basename "$dir")
  zip_path="$out/$name.zip"

  # For a JS plugin: install all deps (the front build needs the
  # devDependencies), run the build script to populate dist/, then prune to
  # RUNTIME deps only. A nodejs backend (main.js) needs its runtime deps (e.g.
  # @grpc/grpc-js) in the shipped node_modules or it crashes at startup with
  # "Cannot find module"; front-end libs are devDependencies (esbuild bundles
  # them into dist/) and must NOT bloat the zip. After the prune, node_modules
  # holds only what the backend needs at runtime.
  # (Side effect: the source tree's node_modules is left pruned; a later local
  #  `npm install` restores the devDeps.)
  if [ -f "$dir/package.json" ]; then
    echo "  $name: installing deps..."
    (cd "$dir" && npm install --silent)
    if grep -q '"build"' "$dir/package.json"; then
      echo "  $name: building front bundle..."
      (cd "$dir" && npm run build --silent)
    fi
    echo "  $name: pruning to runtime deps..."
    (cd "$dir" && npm prune --omit=dev --silent)
  fi

  rm -f "$zip_path"
  # node_modules now holds runtime deps only (post-prune), so it ships; exclude
  # sources and dev config.
  (cd "$dir" && zip -q -X -r "$zip_path" . \
    -x "src/*" \
    -x "tsconfig.json" \
    -x ".gitignore")
  printf "  %-30s -> %s\n" "$name" "$(basename "$zip_path")"
done

echo "done. run 'unzip -l tests/plugins/<name>.zip' to inspect."
