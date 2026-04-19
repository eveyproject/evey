#!/usr/bin/env bash
# Builds the pre-packaged plugin zips under tests/plugins/ from the
# source trees in examples/plugins/. Run this after any example change
# or after a manifest-schema bump so the uploadable bundles stay in
# sync with the documented examples.
#
# Usage: ./scripts/build-test-plugins.sh
#
# Zip names track the source directory name (not the manifest `name:`)
# so a reader browsing examples/plugins/<foo>/ finds the matching
# tests/plugins/<foo>.zip without cross-referencing manifests.
# Security-test + theme-malicious plugins have no example dir; they
# live directly under tests/plugins/ and are left untouched.

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
  rm -f "$zip_path"
  # -j would flatten; we want relative paths preserved.
  # -X strips extra attributes (reproducibility-friendly).
  # -r recurse, -q quiet.
  (cd "$dir" && zip -q -X -r "$zip_path" .)
  printf "  %-30s -> %s\n" "$name" "$(basename "$zip_path")"
done

echo "done. run 'unzip -l tests/plugins/<name>.zip' to inspect."
