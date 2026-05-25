#!/usr/bin/env bash
# Regenerates the Cosign verification test fixtures in this directory.
#
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

# Trust key pair (OpenSSL EC P-256). test.key signs; test.pub is what the server
# trusts via EVEY_PLUGINS_COSIGN_PUBLIC_KEY.
openssl ecparam -genkey -name prime256v1 -noout -out test.key
openssl ec -in test.key -pubout -out test.pub 2>/dev/null

# Sign a blob exactly the way `cosign verify-blob --key <pub>` expects:
# ECDSA over SHA-256, base64-encoded, no trailing newline.
sign() { openssl dgst -sha256 -sign "$1" "$2" | base64 | tr -d '\n' > "$3"; }

# 1. VALID — signed with test.key; verifies under test.pub.
( cd "$work/plugin" && zip -qr "$work/valid.zip" . )
cp "$work/valid.zip" plugin-valid.zip
sign test.key plugin-valid.zip plugin-valid.zip.sig

# 2. TAMPERED — the ZIP is modified AFTER signing, so the signature no longer matches.
cp plugin-valid.zip plugin-tampered.zip
printf 'tamper' >> plugin-tampered.zip
cp plugin-valid.zip.sig plugin-tampered.zip.sig

# 3. WRONGKEY — signed with a different key the server does not trust.
openssl ecparam -genkey -name prime256v1 -noout -out "$work/other.key"
cp plugin-valid.zip plugin-wrongkey.zip
sign "$work/other.key" plugin-wrongkey.zip plugin-wrongkey.zip.sig

echo "Regenerated: test.key test.pub plugin-{valid,tampered,wrongkey}.zip(.sig)"
echo "Verify (needs cosign):"
echo "  cosign verify-blob --key test.pub --signature plugin-valid.zip.sig --insecure-ignore-tlog plugin-valid.zip"
