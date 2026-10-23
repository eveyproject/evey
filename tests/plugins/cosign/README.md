# Signing plugins with Cosign

Evey can require third-party plugin ZIPs to carry a [Cosign](https://github.com/sigstore/cosign)
signature before they install. This directory holds the verification **test fixtures**;
this README is the **author how-to** (every command below is verified against the
server's actual verifier, cosign v2.6.3).

## How the server verifies

The signing **public key travels with the package**: ship it as `cosign.pub` at the
ZIP root. The server (`internal/adapters/driven/pluginrunner/cosign.go`) checks, in-process
and offline, that the detached signature (`<zip>.sig`) matches the package under that key
(`cosign verify-blob --key cosign.pub --signature <zip>.sig --insecure-ignore-tlog <zip>`),
then looks the key's fingerprint (`sha256:` of its DER public key) up in the instance's
trusted-publisher list. The fingerprint is pinned on the plugin at install: an update must
carry the same key, or the admin has to accept the new publisher.

Policy (`evey.yaml: cosign_policy` / env `EVEY_PLUGINS_COSIGN_POLICY`):

| Policy | Behaviour |
|---|---|
| `off` (default) | signature not checked |
| `warn` | verdict recorded + shown to admin, install proceeds |
| `enforce` | install blocked unless the signature verifies AND the key is a trusted publisher |

`EVEY_PLUGINS_COSIGN_PUBLIC_KEY=/path/to/cosign.pub` seeds the operator's own key into the
trusted-publisher list at boot; other keys are added from the admin panel.

## Sign your plugin (recommended: cosign-native)

```bash
# 1. One-time: generate a key pair (set a real password, not empty)
COSIGN_PASSWORD="your-passphrase" cosign generate-key-pair      # -> cosign.key (keep secret), cosign.pub

# 2. Build your plugin ZIP (zip the plugin's files, manifest AND cosign.pub at the root)
cp cosign.pub my-plugin/cosign.pub
cd my-plugin && zip -r ../my-plugin.zip . && cd ..

# 3. Sign the ZIP OFFLINE (no Rekor - matches the server's --insecure-ignore-tlog)
COSIGN_PASSWORD="your-passphrase" cosign sign-blob \
  --key cosign.key --tlog-upload=false --yes \
  --output-signature my-plugin.zip.sig my-plugin.zip

# 4. Sanity-check with the server's exact command
cosign verify-blob --key cosign.pub --signature my-plugin.zip.sig \
  --insecure-ignore-tlog my-plugin.zip          # -> Verified OK
```

Publish `my-plugin.zip.sig` next to the ZIP (a URL install fetches `<url>.sig`) and the key's fingerprint on your project page; keep `cosign.key` secret.

## Upload a signed plugin

The upload endpoint takes the signature as an **optional `sig` multipart field**
(`internal/adapters/driving/http/plugin_asset_handler.go`):

```bash
curl -X POST https://<host>/api/v1/admin/plugins/upload \
  -H "Authorization: Bearer <admin-token>" \
  -F "plugin=@my-plugin.zip" \
  -F "sig=@my-plugin.zip.sig"
```

## Alternative: plain OpenSSL (how the fixtures here were made)

Cosign's `verify-blob` validates any ECDSA-P256-over-SHA256 signature, so you can sign
with OpenSSL and a raw EC key instead of a cosign key:

```bash
openssl ecparam -genkey -name prime256v1 -noout -out test.key   # private (SEC1)
openssl ec -in test.key -pubout -out test.pub                   # public (SPKI)
openssl dgst -sha256 -sign test.key my-plugin.zip | base64 -w0 > my-plugin.zip.sig
cosign verify-blob --key test.pub --signature my-plugin.zip.sig --insecure-ignore-tlog my-plugin.zip
```

## The fixtures in this directory

`test.key`/`test.pub` are an OpenSSL EC key pair (`test.key` is gitignored and generated once per checkout). Every ZIP embeds `test.pub` as `cosign.pub`. Regenerate everything with `./gen-fixtures.sh`.

| Fixture | Signature | `verify-blob` result |
|---|---|---|
| `plugin-valid.zip` (+`.sig`) | OpenSSL-signed with `test.key` | **Verified OK** |
| `plugin-tampered.zip` (+`.sig`) | signed, then the ZIP was modified | **rejected** (invalid signature) |
| `plugin-wrongkey.zip` (+`.sig`) | carries `test.pub` but signed with a *different* key | **rejected** (invalid signature) |
