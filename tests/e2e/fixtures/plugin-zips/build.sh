#!/usr/bin/env bash
# Zips each malicious plugin fixture into a .zip suitable for upload via the
# admin UI or API. Run from the repo root:
#   bash tests/e2e/fixtures/plugin-zips/build.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Navigate up: plugin-zips -> fixtures -> e2e -> tests -> repo-root
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../../.." && pwd)"
SRC="${REPO_ROOT}/tests/fixtures/plugins/malicious"
DST="${SCRIPT_DIR}"

if [[ ! -d "${SRC}" ]]; then
  echo "ERROR: malicious fixtures not found at ${SRC}" >&2
  exit 1
fi

for dir in "${SRC}"/*/; do
  name="$(basename "${dir}")"
  out="${DST}/${name}.zip"
  (cd "${dir}" && zip -r "${out}" .)
  echo "zipped: ${name}.zip"
done
