# Plugin fixture ZIPs (signed)

Prebuilt malicious plugin ZIPs consumed by `tests/e2e/scenarios/admin/plugin-security.spec.ts`.
Each ZIP has a detached cosign signature `*.zip.sig` produced with the
**test-only** keypair `cosign-test.key` / `cosign-test.pub` committed alongside.

## Why a committed test key?

This keypair is **for fixtures only** — it never signs production plugins.
Committing it makes the e2e suite hermetic: anyone who clones the repo can
run the signed-install path without a one-shot key-generation ceremony.

If you ever wire cosign into a real-prod admin install path, use a
different key (HSM, Vault, Sigstore Fulcio, etc.) — never reuse this one.

## Regenerate

```bash
# Rebuild + re-sign every ZIP from tests/fixtures/plugins/malicious/
bash tests/e2e/fixtures/plugin-zips/build.sh
```

## Verify a sig manually

```bash
cosign verify-blob \
  --key tests/e2e/fixtures/plugin-zips/cosign-test.pub \
  --signature tests/e2e/fixtures/plugin-zips/php-string-concat-rce.zip.sig \
  --insecure-ignore-tlog \
  tests/e2e/fixtures/plugin-zips/php-string-concat-rce.zip
```

## Regenerate the keypair (if ever lost)

```bash
cd tests/e2e/fixtures/plugin-zips
COSIGN_PASSWORD="" cosign generate-key-pair --output-key-prefix cosign-test
bash build.sh   # re-sign everything with the new key
```
