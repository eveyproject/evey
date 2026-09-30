# Plugin developer setup

End-to-end walkthrough for someone who has just cloned Evey and wants to write, test, and ship a plugin or theme. Pick the runtime that matches your plugin language; the rest is shared.

## 1. Prerequisites

Evey runs the host process (Go) and ships plugins as ZIPs. The plugin runtime depends on what your code calls.

### Always required

| Tool | Why | Install |
|---|---|---|
| Go ≥ 1.27 | Build the Evey host | `https://go.dev/dl/` |
| Docker + Docker Compose | Default dev stack (Postgres, Meilisearch, server, web) | https://docs.docker.com/engine/install/ |
| `bubblewrap` (`bwrap`) | Sandbox for plugin processes - required for the `bubblewrap` and `strict` modes | `apt install bubblewrap` (Debian/Ubuntu), `dnf install bubblewrap` (Fedora), `pacman -S bubblewrap` (Arch). macOS: install Linux in a VM, bwrap is Linux-only. |

The `basic` sandbox mode runs without bwrap but should never be used outside throwaway local experiments - drop it as soon as you can.

### Per language

Pick what your plugin actually uses; you don't need them all.

| Plugin language | Tools | Notes |
|---|---|---|
| Python 3 | `python3`, `pip` | Use `pip install --user` to keep host clean. SDK stubs live in `sdk/python/`. |
| Node.js | Node 20+, `npm` or `pnpm` | SDK loads `plugin_api.proto` at runtime via `@grpc/proto-loader`. |
| Go | Go ≥ 1.21 | No bundled stubs - generate with `protoc-gen-go` + `protoc-gen-go-grpc`; a compiled Go binary also needs the `native-binary` permission. |
| Dart / Java / PHP / Ruby / Swift | Per-language toolchain | Stubs are pre-generated in `sdk/<lang>/`. |
| CSS-only theme | Nothing extra | A theme is a ZIP with `kind: theme` + a `theme.css`. |
| Theme JS | Nothing extra (the host runs your JS inside a sandboxed `iframe` with `sandbox="allow-scripts"` and a locked-down CSP) | The static scanner refuses cookies, storage, network calls and access to the host page (`document.cookie`, `localStorage`, `fetch(`, `window.parent`...) - see [Themes](../themes.md#what-the-scanner-refuses). |

### Generating stubs for another language

Stubs ship pre-generated for the languages above, but the plugin gRPC contract
is just a proto file - you can target **any language `protoc` supports** (Rust,
C#, Kotlin, Elixir, …). The single source of truth is:

```
proto/plugin/v1/plugin_api.proto
```

Generate with `protoc` (or `buf`), pointing your language's gRPC plugin at it:

```bash
# generic shape - exact --*_out flags depend on the language/plugin
protoc --proto_path=proto \
       --<lang>_out=OUT_DIR --grpc_<lang>_out=OUT_DIR \
       proto/plugin/v1/plugin_api.proto
```

Worked recipes for the bundled languages (which gRPC plugin each needs:
`grpc_php_plugin`, `protoc-gen-grpc-java`, `protoc-gen-grpc-swift`, …) live in
the `plugin-sdk` target of the root `Makefile` and in each `sdk/<lang>/README.md`
(e.g. `sdk/php/README.md`). `make plugin-sdk` regenerates the shipped stubs.

Vendor the generated stubs inside your plugin ZIP - the host only auto-copies
stubs for the languages it ships (`sdk/<lang>/`).

### Optional but recommended

| Tool | Why |
|---|---|
| `cosign` (Sigstore) | Sign your plugin ZIP. `EVEY_PLUGINS_COSIGN_POLICY` defaults to `warn` (not `off`) - with `warn` and no public key configured, verification is a no-op (a warning is logged, nothing is blocked); set `EVEY_PLUGINS_COSIGN_PUBLIC_KEY` to actually verify signatures, and the policy to `enforce` to refuse unsigned uploads (`enforce` without a public key stops the server at startup rather than accept every plugin unverified). https://docs.sigstore.dev/cosign/installation/ |
| `pre-commit` | Catch obvious issues before submitting a plugin. |
| `golangci-lint` | Required if your plugin is in Go and you want linting parity with the host. |

## 2. Bring up Evey locally

```sh
git clone git@github.com:Evey-project/evey.git
cd evey
cp .env.dev.example .env.dev   # generates a sane local config
make dev                        # docker compose up: postgres, meilisearch, server (Air), web (Vite)
```

When the server is healthy, browse to https://evey.test (Caddy serves it). The setup wizard creates the administrator account on first launch, as described in [First setup](../getting-started/first-setup.md).

If `make dev` fails on `bwrap`:
- Confirm `which bwrap` returns a path on the host
- Inside Docker, the dev container ships the bwrap binary; if you customised the Compose stack, bind-mount it or set `EVEY_PLUGINS_SANDBOX_MODE=basic` *for the dev container only*

## 3. Scaffold your first plugin

Examples live in `examples/plugins/`: starters in Node.js and Python (`nodejs-example`, `python-example`), a metadata provider (`metadata-example`), a webhook receiver (`webhook-receiver`), a home widget (`weather`), plugin configuration (`config-demo`) and two themes (`theme-dark-blue`, `open-workshop`). Pick the closest one to your use case and copy it into a working directory. For the persistence + permission features, see **`examples/plugins/data-demo`** - it uses the per-plugin data store and requests the `public` / `prefs` permissions + the `admin.nav` slot.

```sh
cp -r examples/plugins/nodejs-example ~/my-first-plugin
cd ~/my-first-plugin
```

Edit `plugin.yaml`:

- `name`: kebab-case, unique
- `version`: semver - bump on every release
- `kind`: `plugin` (backend required) or `theme` (frontend-only)
- `language`: `python` | `nodejs` | `go` | `php` | `ruby` | `lua` | `bash` | `sh` (manifest identifier - not the interpreter binary name; `python3`/`node` are not valid `language` values, see `entrypoint` below).
- `entrypoint`: command line the runtime executes
- `permissions`: declare every host API you call. The installer rejects calls that aren't declared. See the [permissions catalog](security-model.md#permissions-catalog) for the exhaustive list.
- `external_fetch`: when you need to talk to a third-party API. Entries are bare hostnames (`api.example.com`) - no scheme required or used (any `https://` prefix is parsed and only the hostname is kept), no wildcards (`*.example.com` is rejected at install: `external_fetch entry %q is a wildcard - use explicit hostnames`), no literal IP addresses (all of them, not just RFC1918). The validator lives in `internal/adapters/driven/pluginrunner/manifest.go` (`validateExternalFetch`); its test coverage is in `internal/adapters/driven/pluginrunner/manifest_test.go` - run `go test ./internal/adapters/driven/pluginrunner/...` to check your value matches.

Frontend module (optional for `kind: plugin`, banned for `kind: theme`):

```yaml title="plugin.yaml"
frontend:
  - slot: home.section
    src: dist/index.js
```

The slot must exist in `internal/domain/plugin/slots.go` (`home.section`, `home.hero`, `library.sidebar`, `media.detail.sidebar`, `player.overlay`, `sidebar.bottom`, `settings.user.appearance`, and `admin.nav` - the one controlled admin-area slot, a menu entry opening your plugin's own sandboxed page). Anything else (`admin.*` injection, `login.*`, `setup.*`, `profile.security.*`) is rejected at install time.

**Storing data.** Your backend gets a private key/value store over the host
socket (`SetData`/`GetData`/`DeleteData`/`ListDataKeys`) - no permission needed,
wiped automatically on uninstall. For per-user preferences from your iframe,
request the `prefs` permission; for read-only access to non-sensitive media
(title/poster), request `public`. See the [API reference](api-reference.md#permissions). Admin,
auth, system and generic write scopes can never be requested.

## 4. Run the install loop

```sh
# zip your plugin
cd ~/my-first-plugin
zip -r my-first-plugin.zip . -x ".*" "node_modules/*" "*.pyc"

# upload through the admin UI
open https://evey.test/admin/plugins
# or via the API
curl -F "plugin=@my-first-plugin.zip" \
     -H "Authorization: Bearer $EVEY_TOKEN" \
     https://evey.test/api/v1/admin/plugins/upload
# a signature goes in a second field: -F "sig=@my-first-plugin.zip.sig"
# an update that asks for new permissions: -F "accept_permission_changes=true"
```

The installer runs in this order - every step can refuse the install:

1. **Manifest validation** - `plugin.yaml` parsed straight out of the ZIP (before any ZIP-level checks below), schema-checked. Slot whitelist. Permission/capability whitelist. `external_fetch` parser (rejects wildcards and any literal IP address; a scheme prefix like `http://` is accepted but ignored - only the hostname is kept). The declared `language` runtime must be available in the image.
2. **Permission escalation** (update only) - the installer compares the new manifest against the previous version. Any newly-requested permission/capability/slot/external_fetch host blocks the install with an error the caller must explicitly override (`accept_permission_changes: true` from the frontend). Fresh installs skip this step - there's nothing to compare against.
3. **Cosign signature** - verified when a signature verifier is configured; a failure only blocks the install when `EVEY_PLUGINS_COSIGN_POLICY=enforce`. See section 6.
4. **ZIP extraction** - extension, total size ≤ 500 MB, per-entry size ≤ 100 MB, compression ratio ≤ 100:1, no symlinks, no path-traversal entries; files land in a staging dir (upgrades never touch the live version until every later step passes).
5. **CSS sanitizer** (themes only) - strips `@import`, blocks `url()` non-`data:`, blocks fullscreen overlays, blocks selectors targeting `[type=password]` and `.admin-*`, clamps `z-index` ≤ 999.
6. **Theme JS scanner** (themes only) - refuses any reference to `document.cookie`, `localStorage`, `fetch`, `eval`, `globalThis`, bracket-notation property access on host globals, `constructor.constructor`, etc. Themes must use only the `evey.theme.*` API.
7. **Static analyzer** - scans every text file for shell exec, reverse shell, eval, base64-decode-then-exec, obfuscation patterns, runtime install commands, privilege escalation. Critical findings = refuse install. Warning findings (deserialization, obfuscation, native_code) escalate to refuse unless the matching permission is declared.
8. **Persist** - swaps the staging dir into place, writes the row in Postgres, optionally launches the runtime.

If any step fails, the admin UI shows the exact reason. Server logs (`docker compose logs evey`) carry the full stack.

## 5. Iterate

Hot-reload your plugin while developing:

```sh
# edit code → re-zip → re-upload via UI (Update button)
# or, faster: drop the file directly into the plugin directory inside the container
docker compose exec evey ls /data/plugins/<plugin-id>/
```

An upload is accepted as an update only when its `version` is higher than the installed one: bump it on every iteration (a same or lower version is refused), or uninstall first. If you add a new permission, you'll hit the escalation flow - that's the design, not a bug.

Logs from the plugin process land in:
- `/admin/plugins/<id>` - last N lines via the UI
- `docker compose logs evey | grep <plugin-id>` - full stream

## 6. Sign your plugin (optional in dev, recommended for distribution)

If you plan to distribute the plugin to other Evey instances, sign it. The receiving admin can then run `EVEY_PLUGINS_COSIGN_POLICY=warn` (or `enforce`) to surface unsigned uploads.

```sh
# one-time: generate a keypair (keep cosign.key safe - never commit it)
cosign generate-key-pair

# sign the ZIP
cosign sign-blob \
  --key cosign.key \
  --output-signature my-first-plugin.zip.sig \
  my-first-plugin.zip

# distribute: my-first-plugin.zip + my-first-plugin.zip.sig + cosign.pub
```

On the Evey instance that installs the plugin:

```sh
# put the public key somewhere readable by the server
sudo cp cosign.pub /etc/evey/cosign.pub

# point Evey at it and switch policy
export EVEY_PLUGINS_COSIGN_PUBLIC_KEY=/etc/evey/cosign.pub
export EVEY_PLUGINS_COSIGN_POLICY=warn   # or "enforce" to refuse unsigned
docker compose restart evey
```

Evey validates the key at startup (PEM parse) - a missing or malformed key file fails the boot rather than the first install. KMS references (`gcpkms://...`, `awskms://...`, `k8s://...`) are accepted opaquely.

## 7. Resource limits and sandbox

Defaults applied to every plugin process:

| Env var | Default | Effect |
|---|---|---|
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` | `auto` = picks best available (normally `landlock`). `landlock` = Landlock LSM allowlist + unconditional TCP egress denial (`-restrict-net` + seccomp `-deny-inet`, no carve-out for the `network` permission) + LD_PRELOAD seccomp. `basic` = LD_PRELOAD seccomp + UID drop only (debug). `bubblewrap` = full namespace + bind-mount root readonly + `--unshare-net` (also unconditional, every plugin, regardless of the `network` permission). `strict` = bubblewrap + the LD_PRELOAD seccomp shim. |
| `EVEY_PLUGINS_MEMORY_MB` | 256 | Max RSS. The resource monitor polls `/proc` and stops the plugin past this. `0` disables. |
| `EVEY_PLUGINS_CPU_NICE` | 10 | `nice` value (0-19) applied at spawn so a plugin can never starve the server of CPU. |
| `EVEY_PLUGINS_PIDS_MAX` | 64 | Max thread/process count. Prevents fork bombs. `0` disables. |
| `EVEY_PLUGINS_DISK_MAX_MB` | 100 | Max install-directory size; the monitor measures it every ~30s and stops a plugin past it. `0` disables. |

A plugin that talks to the network needs both `permissions: [network]` and `external_fetch: [<host>...]`. Direct network egress is cut at the sandbox for every plugin, unconditionally, regardless of its permissions (see the sandbox modes above) - the plugin process itself can never open a socket to the internet. The only way out is the host-side `Fetch` RPC broker: the plugin calls it over the `EVEY_PLUGIN_SOCK` Unix socket, the host checks the target host against `external_fetch`, then dials through an SSRF-safe client (private/reserved IPs rejected at connect time, redirects re-validated against both the resolved IP and the allow-listed host). See `internal/adapters/driving/pluginhost/fetch.go`.

## 8. Distribute a plugin

There is no official catalog yet: an admin installs a plugin by uploading its
ZIP in the admin panel, from wherever you publish it (a GitHub release, your
own site). What "distribution" means today:

- Ship the ZIP, its `.sig` and your `cosign.pub` together (section 6), so the
  receiving admin can verify the upload with `EVEY_PLUGINS_COSIGN_POLICY=warn`
  or `enforce`.
- State the permissions and `external_fetch` hosts in your README - the admin
  sees them at install time and has to approve them; a surprise there is the
  first reason an install gets refused.
- To have your plugin listed with the bundled examples, open a PR against
  `examples/plugins/` with the source and a README. Examples are reviewed and
  built from source; a PR is not an endorsement and carries no signature from
  the project.

## 9. Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `bwrap: ... permission denied` | `bubblewrap` mode inside Docker: the default seccomp and AppArmor profiles block the `unshare` and `mount` calls it needs. | Keep the default `auto` mode, which uses Landlock in a container. Otherwise apply the bundled profile `docker/seccomp-evey.json` (also in the image at `/etc/evey/seccomp.json`), a deliberate security downgrade: see [Plugin sandbox setup](sandbox-deployment.md). |
| `forbidden pattern "fetch(" found in theme JS` | Theme JS scanner caught a banned API. | Themes must only call `evey.theme.*`. Read `internal/pkg/themejs/scan.go` to see what's blocked. |
| `manifest: external_fetch entry "*.tmdb.org" is a wildcard - use explicit hostnames` | All wildcards are rejected at install, no exceptions. | Use `api.tmdb.org` (and list every explicit subdomain you need), not `*.tmdb.org` or `cdn.*.tmdb.org`. |
| `manifest: frontend slot "admin.section" not allowed` | Plugin frontends are banned from privileged areas. | Pick a non-critical slot from the whitelist or rethink the plugin. |
| `cosign verify-blob: failed signature` | Public key on the server doesn't match the private key used to sign. | Re-sign with the right key, or update `EVEY_PLUGINS_COSIGN_PUBLIC_KEY`. |
| `plugin already installed at version 1.0.0, new version must be higher` | The ZIP carries the version already installed. | Bump `version` in `plugin.yaml`, or uninstall the plugin first. |
| Plugin stops, the log says `memory quota exceeded (… MB > … MB)` | The resource monitor killed it past its memory limit (exit 137). | Lower memory usage, or raise the plugin's memory limit on its card in **Admin → Plugins** (`EVEY_PLUGINS_MEMORY_MB` sets the default). |
| Frontend module not visible | Plugin not enabled, slot mismatch, or `web_root` prefix wrong in `frontend.src`. | Check the admin Plugins page (status badge), confirm the slot string matches a real `<PluginSlot name=...>` mount, look at the network panel for `/api/v1/plugins/{id}/assets/...` calls. |

## 10. Going further

- Security model end-to-end: [security-model.md](security-model.md)
- Host API surface: [api-reference.md](api-reference.md)
- Available slots: [api-reference.md](api-reference.md#slots)
- SDK code per language: `sdk/`
- Reference plugins and themes: `examples/plugins/`
