#!/usr/bin/env bash
# Zips each malicious plugin fixture into a .zip and signs each with the
# committed cosign test key (cosign-test.key / cosign-test.pub). Run from
# the repo root:
#   bash tests/e2e/fixtures/plugin-zips/build.sh
#
# Re-running is idempotent: ZIP+sig are regenerated. cosign must be in $PATH.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../../.." && pwd)"
SRC="${REPO_ROOT}/tests/fixtures/plugins/malicious"
DST="${SCRIPT_DIR}"
KEY="${DST}/cosign-test.key"

if [[ ! -d "${SRC}" ]]; then
  echo "ERROR: malicious fixtures not found at ${SRC}" >&2
  exit 1
fi
if ! command -v cosign >/dev/null 2>&1; then
  echo "ERROR: cosign not in \$PATH — install via 'go install github.com/sigstore/cosign/v2/cmd/cosign@latest'" >&2
  exit 1
fi
if [[ ! -f "${KEY}" ]]; then
  echo "ERROR: ${KEY} missing — regenerate with 'COSIGN_PASSWORD= cosign generate-key-pair --output-key-prefix cosign-test'" >&2
  exit 1
fi

for dir in "${SRC}"/*/; do
  name="$(basename "${dir}")"
  out="${DST}/${name}.zip"
  rm -f "${out}" "${out}.sig"
  (cd "${dir}" && zip -qr "${out}" .)
  COSIGN_PASSWORD="" cosign sign-blob \
    --key "${KEY}" \
    --output-signature "${out}.sig" \
    --tlog-upload=false \
    --yes \
    "${out}" >/dev/null 2>&1
  echo "zipped+signed: ${name}.zip"
done
