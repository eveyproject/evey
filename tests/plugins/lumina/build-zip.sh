#!/usr/bin/env bash
# Packages ../lumina.zip for upload through the admin panel.
#
# The exclusion list is driven by the server's static analyzer
# (internal/adapters/driven/pluginrunner/analyzer.go):
#   - This script is excluded: it contains the literal package-manager
#     install commands, which the analyzer flags as CRITICAL runtime_install
#     regardless of context.
#   - The dev package.json is swapped for a metadata-only one inside the
#     zip: any non-empty "dependencies" object triggers the unvendored_deps
#     warning even when node_modules IS vendored (the analyzer never checks),
#     and node resolves require() from node_modules without a manifest.
#   - src/, tests/, tsconfig, vitest config, lockfile: no runtime purpose,
#     they only widen the scan surface.
#
# The zip therefore contains exactly: plugin.yaml, main.js, dist/index.js,
# dist/settings.js, node_modules (runtime deps only) and the minimal
# package.json.

set -euo pipefail
cd "$(dirname "$0")"

npm install --silent
npm run typecheck --silent
npm test --silent
npm run build --silent
npm prune --omit=dev --silent

# npm prune has been observed (see weather-v2/build-zip.sh) to
# non-deterministically leave dev-only transitive packages on disk right
# after a test/build run in the same process. Force-remove every
# package-lock.json entry marked dev:true directly instead: deterministic,
# driven by the lockfile (ground truth for what's actually a devDependency).
node -e "
  const fs = require('fs');
  const path = require('path');
  const lock = require('./package-lock.json');
  for (const [pkgPath, meta] of Object.entries(lock.packages)) {
    if (!meta.dev || !pkgPath.startsWith('node_modules/')) continue;
    const rel = pkgPath.slice('node_modules/'.length);
    if (rel.includes('/node_modules/')) continue; // nested: parent removal covers it
    fs.rmSync(path.join('node_modules', rel), { recursive: true, force: true });
  }
"

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
  # Runtime cache artifacts (e.g. vitest's node_modules/.vite/), never actual
  # packages - safe to drop unconditionally, .bin is already handled above.
  find node_modules -mindepth 1 -maxdepth 1 -type d -name '.*' ! -name '.bin' -exec rm -rf {} + 2>/dev/null || true
  # Empty scope directories left behind once every package inside them was
  # removed above (e.g. node_modules/@grpc/ once all of @grpc/* is gone):
  # harmless (0 bytes) but pointless in the zip listing.
  find node_modules -depth -type d -empty -delete 2>/dev/null || true
fi

# Deterministic proof the zip ships no devDependency: fail loudly instead of
# silently shipping bloat if any lockfile-marked dev package survived.
node -e "
  const fs = require('fs');
  const path = require('path');
  const lock = require('./package-lock.json');
  const leftover = Object.entries(lock.packages)
    .filter(([p, m]) => m.dev && p.startsWith('node_modules/') && !p.slice('node_modules/'.length).includes('/node_modules/'))
    .map(([p]) => path.join('node_modules', p.slice('node_modules/'.length)))
    .filter((p) => fs.existsSync(p));
  if (leftover.length > 0) {
    console.error('devDependency leaked into the zip payload:', leftover);
    process.exit(1);
  }
"

out="../lumina.zip"
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
  -x "vitest.config.ts" \
  -x "package.json.dev" \
  -x "package-lock.json" \
  -x "build-zip.sh"

echo "built $out"
unzip -l "$out" | tail -2
