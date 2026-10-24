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
# node_modules (runtime deps only), the minimal package.json and, when the
# build is signed, cosign.pub.
#
# Signing (optional): SIGN_KEY=<private PEM> SIGN_PUB=<public PEM> ./build-zip.sh
# copies the public key into the ZIP as cosign.pub and writes
# ../weather-v2.zip.sig, the detached signature Evey verifies (ECDSA P-256 over
# the SHA-256 of the ZIP, what `cosign sign-blob --key` produces; openssl makes
# the same bytes). Publish the .sig next to the ZIP. Without the two variables
# the build is unsigned, as before.

set -euo pipefail
cd "$(dirname "$0")"

npm install --silent
npm run typecheck --silent
npm test --silent
npm run build --silent
npm prune --omit=dev --silent

# npm prune has been observed to non-deterministically leave dev-only
# transitive packages on disk right after a test/build run in the same
# process (vitest/esbuild/rollup and their platform binaries, ~3MB) even
# though package-lock.json correctly marks them dev:true and a prune re-run
# later (in a fresh shell) does clean them up. Rather than depend on npm's
# flaky convergence, force-remove every package-lock.json entry marked
# dev:true directly: this is deterministic and driven by the lockfile
# (ground truth for what's actually a devDependency), not a guessed name list.
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
  # removed above (e.g. node_modules/@vitest/ once all of @vitest/* is gone):
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

out="../weather-v2.zip"
rm -f "$out" "$out.sig" cosign.pub
if [ -n "${SIGN_KEY:-}" ] || [ -n "${SIGN_PUB:-}" ]; then
  [ -n "${SIGN_KEY:-}" ] && [ -n "${SIGN_PUB:-}" ] || { echo "SIGN_KEY and SIGN_PUB go together" >&2; exit 1; }
  cp "$SIGN_PUB" cosign.pub
fi

cp package.json package.json.dev
node -e "
  const p = require('./package.json');
  require('fs').writeFileSync('package.json',
    JSON.stringify({ name: p.name, version: p.version, private: true }, null, 2) + '\n');
"
trap 'mv package.json.dev package.json; rm -f cosign.pub' EXIT

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

if [ -n "${SIGN_KEY:-}" ]; then
  openssl dgst -sha256 -sign "$SIGN_KEY" "$out" | base64 | tr -d '\n' > "$out.sig"
  echo "signed $out.sig (key $(openssl pkey -pubin -in "$SIGN_PUB" -outform DER | sha256sum | cut -c1-16)...)"
fi

echo "built $out"
unzip -l "$out" | tail -2
