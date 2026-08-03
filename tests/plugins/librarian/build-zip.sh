#!/usr/bin/env bash
# Packages ../librarian.zip for upload through the admin panel.
#
# Mirrors weather-v2's recipe and exclusions (driven by the server's static
# analyzer): README.md and this script stay out of the zip (they carry the
# literal install command the analyzer flags), the dev package.json is
# swapped for a metadata-only one (a non-empty "dependencies" object trips
# unvendored_deps even when node_modules IS vendored), and tests/ has no
# runtime purpose.
#
# The zip contains exactly: plugin.yaml, plugin_api.proto, main.js, dist/,
# node_modules (runtime deps only) and the minimal package.json.

set -euo pipefail
cd "$(dirname "$0")"

npm install --silent
npm test --silent

# Deterministic dev-dependency removal, lockfile-driven (see weather-v2's
# build-zip.sh for why npm prune alone is not trusted).
npm prune --omit=dev --silent
node -e "
  const fs = require('fs');
  const path = require('path');
  const lock = JSON.parse(fs.readFileSync('./package-lock.json', 'utf8'));
  for (const [pkgPath, meta] of Object.entries(lock.packages)) {
    if (!meta.dev || !pkgPath.startsWith('node_modules/')) continue;
    const rel = pkgPath.slice('node_modules/'.length);
    if (rel.includes('/node_modules/')) continue;
    fs.rmSync(path.join('node_modules', rel), { recursive: true, force: true });
  }
"

cp package.json package.json.dev
node -e "
  const fs = require('fs');
  const p = JSON.parse(fs.readFileSync('./package.json.dev', 'utf8'));
  fs.writeFileSync('package.json', JSON.stringify({ name: p.name, version: p.version, private: true }, null, 2));
"

rm -f ../librarian.zip
zip -qr ../librarian.zip plugin.yaml plugin_api.proto main.js dist node_modules package.json

mv package.json.dev package.json
echo "built ../librarian.zip"
unzip -l ../librarian.zip | tail -2
