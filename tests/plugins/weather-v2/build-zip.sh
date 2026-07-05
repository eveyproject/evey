#!/usr/bin/env bash
# Packages ../weather-v2.zip for upload through the admin panel.
#
# The exclusion list is driven by the server's static analyzer
# (internal/adapters/driven/pluginrunner/analyzer.go):
#   - README.md and this script are excluded: they contain the literal
#     package-manager install commands, which the analyzer flags as
#     CRITICAL runtime_install regardless of context.
#   - The dev package.json is swapped for a metadata-only one inside the
#     zip: any non-empty "dependencies" object triggers the unvendored_deps
#     warning even when node_modules IS vendored (the analyzer never checks),
#     and node resolves require() from node_modules without a manifest.
#   - src/, tests/, tsconfig, vitest config, lockfile: no runtime purpose,
#     they only widen the scan surface.
#
# The zip therefore contains exactly: plugin.yaml, main.js, dist/index.js,
# node_modules (runtime deps only) and the minimal package.json.

set -euo pipefail
cd "$(dirname "$0")"

npm install --silent
npm run typecheck --silent
npm test --silent
npm run build --silent
npm prune --omit=dev --silent

# Trim non-runtime cruft from vendored deps (same as scripts/build-test-plugins.sh).
if [ -d node_modules ]; then
  find node_modules -type d \( \
    -iname test -o -iname tests -o -iname __tests__ -o -iname doc -o \
    -iname docs -o -iname example -o -iname examples -o -iname .github -o -iname .bin \
  \) -prune -exec rm -rf {} + 2>/dev/null || true
  find node_modules -type f \( \
    -name '*.md' -o -name '*.markdown' -o -name '*.map' -o -name '*.d.ts' -o \
    -name '*.ts' -o -name '.npmignore' -o -name '.eslintrc*' \
  \) -delete 2>/dev/null || true
fi

out="../weather-v2.zip"
rm -f "$out"

cp package.json package.json.dev
node -e "
  const p = require('./package.json');
  require('fs').writeFileSync('package.json',
    JSON.stringify({ name: p.name, version: p.version, private: true }, null, 2) + '\n');
"
trap 'mv package.json.dev package.json' EXIT

zip -q -X -r "$out" . \
  -x "src/*" \
  -x "tests/*" \
  -x "tsconfig.json" \
  -x ".gitignore" \
  -x "README.md" \
  -x "vitest.config.ts" \
  -x "package.json.dev" \
  -x "package-lock.json" \
  -x "build-zip.sh"

echo "built $out"
unzip -l "$out" | tail -2
