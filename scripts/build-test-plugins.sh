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

  # If the plugin has a package.json with a build script (front bundle),
  # install deps + run build to populate dist/. Skip if dist is fresh.
  if [ -f "$dir/package.json" ] && grep -q '"build"' "$dir/package.json"; then
    if [ ! -d "$dir/node_modules" ]; then
      echo "  $name: installing deps (first run)..."
      (cd "$dir" && npm install --silent)
    fi
    echo "  $name: building front bundle..."
    (cd "$dir" && npm run build --silent)
  fi

  rm -f "$zip_path"
  # Exclude dev artefacts that don't belong in the runtime zip.
  (cd "$dir" && zip -q -X -r "$zip_path" . \
    -x "node_modules/*" \
    -x "src/*" \
    -x "tsconfig.json" \
    -x ".gitignore")
  printf "  %-30s -> %s\n" "$name" "$(basename "$zip_path")"
done

echo "done. run 'unzip -l tests/plugins/<name>.zip' to inspect."
