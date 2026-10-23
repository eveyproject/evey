#!/usr/bin/env bash
# Regenerates the Cosign verification test fixtures in this directory.
#
# Every package embeds its signing public key as cosign.pub at the ZIP root.
# Doubles as a concrete, runnable example of signing a plugin so the server's
# verifier accepts it. Verified against cosign v2.6.3. Needs: openssl, zip.
# (See README.md for the recommended cosign-native signing flow for real plugins.)
set -euo pipefail
cd "$(dirname "$0")"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# A minimal, valid plugin payload: manifest at the ZIP root + entrypoint.
mkdir -p "$work/plugin"
cat > "$work/plugin/plugin.yaml" <<'YAML'
name: plugin-cosign-valid
version: 1.0.0
language: python
entrypoint: main.py
YAML
printf 'print("hello from a signed plugin")\n' > "$work/plugin/main.py"

# Trust key pair (OpenSSL EC P-256). test.key signs; test.pub is the public key
# every package carries as cosign.pub at its root. test.key is gitignored: it is
# generated once per checkout and reused afterwards, so test.pub (tracked) only
# changes when the key does.
if [ ! -f test.key ]; then
  openssl ecparam -genkey -name prime256v1 -noout -out test.key
fi
openssl ec -in test.key -pubout -out test.pub 2>/dev/null

# Sign a blob exactly the way `cosign verify-blob --key <pub>` expects:
# ECDSA over SHA-256, base64-encoded, no trailing newline.
sign() { openssl dgst -sha256 -sign "$1" "$2" | base64 | tr -d '\n' > "$3"; }

# The signing key travels WITH the package.
cp test.pub "$work/plugin/cosign.pub"

# 1. VALID - carries test.pub, signed with test.key; verifies under its own key.
( cd "$work/plugin" && zip -qr "$work/valid.zip" . )
cp "$work/valid.zip" plugin-valid.zip
sign test.key plugin-valid.zip plugin-valid.zip.sig

# 2. TAMPERED - the package content changes AFTER signing (same cosign.pub, the
# original signature), so the signature no longer matches.
printf 'print("tampered")\n' > "$work/plugin/main.py"
( cd "$work/plugin" && zip -qr "$work/tampered.zip" . )
cp "$work/tampered.zip" plugin-tampered.zip
cp plugin-valid.zip.sig plugin-tampered.zip.sig

# 3. WRONGKEY - carries test.pub but was signed with a different key.
openssl ecparam -genkey -name prime256v1 -noout -out "$work/other.key"
cp plugin-valid.zip plugin-wrongkey.zip
sign "$work/other.key" plugin-wrongkey.zip plugin-wrongkey.zip.sig

echo "Regenerated: test.key (kept if present) test.pub plugin-{valid,tampered,wrongkey}.zip(.sig)"
echo "Verify (needs cosign):"
echo "  cosign verify-blob --key test.pub --signature plugin-valid.zip.sig --insecure-ignore-tlog plugin-valid.zip"
