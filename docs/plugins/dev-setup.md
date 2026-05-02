# Plugin developer setup

End-to-end walkthrough for someone who has just cloned Evey and wants to write, test, and ship a plugin or theme. Pick the runtime that matches your plugin language; the rest is shared.

## 1. Prerequisites

Evey runs the host process (Go) and ships plugins as ZIPs. The plugin runtime depends on what your code calls.

### Always required

| Tool | Why | Install |
|---|---|---|
| Go ≥ 1.25 | Build the Evey host | `https://go.dev/dl/` |
| Docker + Docker Compose | Default dev stack (Postgres, Meilisearch, server, web) | https://docs.docker.com/engine/install/ |
| `bubblewrap` (`bwrap`) | Sandbox for plugin processes — required for the `bubblewrap` and `strict` modes | `apt install bubblewrap` (Debian/Ubuntu), `dnf install bubblewrap` (Fedora), `pacman -S bubblewrap` (Arch). macOS: install Linux in a VM, bwrap is Linux-only. |

The `basic` sandbox mode runs without bwrap but should never be used outside throwaway local experiments — drop it as soon as you can.

### Per language

Pick what your plugin actually uses; you don't need them all.

| Plugin language | Tools | Notes |
|---|---|---|
| Python 3 | `python3`, `pip` | Use `pip install --user` to keep host clean. SDK stubs live in `sdk/python/`. |
| Node.js | Node 20+, `npm` or `pnpm` | SDK loads `plugin_api.proto` at runtime via `@grpc/proto-loader`. |
| Go | Go ≥ 1.21 | SDK stubs in `sdk/go/`. |
| Dart / Java / PHP / Ruby / Swift | Per-language toolchain | Stubs are pre-generated in `sdk/<lang>/`. |
| CSS-only theme | Nothing extra | A theme is a ZIP with `kind: theme` + a `theme.css`. |
| Theme JS | Nothing extra (the host runs your JS in a constrained ShadowRoot) | The static scanner refuses any access to `document`, `window`, `fetch`, `localStorage`, etc. — see `internal/pkg/themejs/scan.go`. |

### Optional but recommended

| Tool | Why |
|---|---|
| `cosign` (Sigstore) | Sign your plugin ZIP. Required if your target Evey instance runs `EVEY_PLUGINS_COSIGN_POLICY=warn` (default-off in dev, recommended `warn` in shared prod). https://docs.sigstore.dev/cosign/installation/ |
| `pre-commit` | Catch obvious issues before submitting a plugin. |
| `golangci-lint` | Required if your plugin is in Go and you want linting parity with the host. |

## 2. Bring up Evey locally

```sh
git clone git@github.com:Evey-project/evey.git
cd evey
cp .env.dev.example .env.dev   # generates a sane local config
make dev                        # docker compose up: postgres, meilisearch, server (Air), web (Vite)
```

When the server is healthy, browse to https://evey.test (Caddy serves it). Default credentials are seeded in `.env.dev` — change them before exposing the instance.

If `make dev` fails on `bwrap`:
- Confirm `which bwrap` returns a path on the host
- Inside Docker, the dev container ships the bwrap binary; if you customised the Compose stack, bind-mount it or set `EVEY_PLUGINS_SANDBOX_MODE=basic` *for the dev container only*

## 3. Scaffold your first plugin

Examples live in `examples/plugins/` and `examples/themes/`. Pick the closest one to your use case (TMDB enricher, Discord notifier, Spotify now-playing, weather, dark theme, etc.) and copy it into a working directory:

```sh
cp -r examples/plugins/discord-notify ~/my-first-plugin
cd ~/my-first-plugin
```

Edit `plugin.yaml`:

- `name`: kebab-case, unique
- `version`: semver — bump on every release
- `kind`: `plugin` (backend required) or `theme` (frontend-only)
- `language`: `python3`, `node`, `go`, etc.
- `entrypoint`: command line the runtime executes
- `permissions`: declare every host API you call. The installer rejects calls that aren't declared. See `docs/plugins.md#security-model` for the exhaustive list.
- `external_fetch`: when you need to talk to a third-party API. The host enforces this list at the DNS resolver level inside the plugin's namespace. Strict format: `https://` only, host concrete on TLD+SLD, at most one wildcard (leftmost). Run the validator with `go test ./internal/pkg/extfetch/...` to check your value matches.

Frontend module (optional for `kind: plugin`, banned for `kind: theme`):

```yaml
frontend:
  - slot: home.section
    src: dist/index.js
```

The slot must exist in `internal/domain/plugin/slots.go` (`home.section`, `media.detail.sidebar`, `player.overlay`, `sidebar.bottom`, `settings.user.appearance`). Anything else (`admin.*`, `login.*`, `setup.*`, `profile.security.*`) is rejected at install time.

## 4. Run the install loop

```sh
# zip your plugin
cd ~/my-first-plugin
zip -r my-first-plugin.zip . -x ".*" "node_modules/*" "*.pyc"

# upload through the admin UI
open https://evey.test/admin/plugins
# or via the API
curl -F "file=@my-first-plugin.zip" \
     -H "Authorization: Bearer $EVEY_TOKEN" \
     https://evey.test/api/v1/admin/plugins
```

The installer runs in this order — every step can refuse the install:

1. **ZIP integrity** — extension, total size ≤ 500 MB, per-entry size ≤ 100 MB, compression ratio ≤ 100:1, no symlinks, no path-traversal entries.
2. **Cosign signature** — only when `EVEY_PLUGINS_COSIGN_POLICY != off`. See section 6.
3. **Manifest validation** — `plugin.yaml` parsed, schema-checked. Slot whitelist. Permission/capability whitelist. `external_fetch` parser (rejects `*`, RFC1918, http://, etc.).
4. **Static analyzer** — scans every text file for shell exec, reverse shell, eval, base64-decode-then-exec, obfuscation patterns, runtime install commands, privilege escalation. Critical findings = refuse install. Warning findings (deserialization, obfuscation, native_code) escalate to refuse unless the matching permission is declared.
5. **CSS sanitizer** (themes only) — strips `@import`, blocks `url()` non-`data:`, blocks fullscreen overlays, blocks selectors targeting `[type=password]` and `.admin-*`, clamps `z-index` ≤ 999.
6. **Theme JS scanner** (themes only) — refuses any reference to `document.cookie`, `localStorage`, `fetch`, `eval`, `globalThis`, bracket-notation property access on host globals, `constructor.constructor`, etc. Themes must use only the `evey.theme.*` API.
7. **Permission escalation** — on update, the installer compares the new manifest against the previous version. Any newly-requested permission/capability/slot/external_fetch host blocks the install with HTTP `428 Precondition Required`. The frontend prompts the admin to ack the diff and resubmits with `accept_permission_changes: true`.
8. **Persist** — copies files to the plugin directory, writes the row in Postgres, optionally launches the runtime.

If any step fails, the admin UI shows the exact reason. Server logs (`docker compose logs evey`) carry the full stack.

## 5. Iterate

Hot-reload your plugin while developing:

```sh
# edit code → re-zip → re-upload via UI (Update button)
# or, faster: drop the file directly into the plugin directory inside the container
docker compose exec evey ls /var/lib/evey/plugins/<plugin-id>/
```

When you bump `version`, the installer accepts the upload as an update. If you add a new permission, you'll hit the escalation flow — that's the design, not a bug.

Logs from the plugin process land in:
- `/admin/plugins/<id>` — last N lines via the UI
- `docker compose logs evey | grep <plugin-id>` — full stream

## 6. Sign your plugin (optional in dev, recommended for distribution)

If you plan to distribute the plugin to other Evey instances, sign it. The receiving admin can then run `EVEY_PLUGINS_COSIGN_POLICY=warn` (or `enforce`) to surface unsigned uploads.

```sh
# one-time: generate a keypair (keep cosign.key safe — never commit it)
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

Evey validates the key at startup (PEM parse) — a missing or malformed key file fails the boot rather than the first install. KMS references (`gcpkms://...`, `awskms://...`, `k8s://...`) are accepted opaquely.

## 7. Resource limits and sandbox

Defaults applied to every plugin process:

| Env var | Default | Effect |
|---|---|---|
| `EVEY_PLUGINS_SANDBOX_MODE` | `bubblewrap` | `basic` = fork only (debug). `bubblewrap` = full namespace + bind-mount root readonly + `--unshare-net` unless `network` permission. `strict` = adds seccomp profile. |
| `EVEY_PLUGINS_MEMORY_MB` | 256 | Hard cgroup memory cap. Plugin gets OOM-killed past this. |
| `EVEY_PLUGINS_CPU_PERCENT` | 50 | cgroup CPU quota (fraction of one core). |
| `EVEY_PLUGINS_PIDS_MAX` | 64 | Max process count (cgroup pids controller). Prevents fork bombs. |

A plugin that talks to the network needs both `permissions: [network]` and `external_fetch: [<host>...]`. Hosts not in the whitelist NXDOMAIN inside the plugin's namespace; literal IP connect bypasses the DNS layer (V1 limitation, fixed in V2 with nftables).

## 8. Submit a plugin to the official catalog

(Process TBD — placeholder.) Open a PR against `examples/plugins/` with your plugin source and a README. Maintainers review and re-sign with the official key before publishing.

## 9. Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `bwrap: ... permission denied` | Default Docker seccomp blocks `unshare`. | The repo ships a custom seccomp profile (`docker/seccomp.json`). Make sure your compose file applies it. |
| `forbidden pattern "fetch(" found in theme JS` | Theme JS scanner caught a banned API. | Themes must only call `evey.theme.*`. Read `internal/pkg/themejs/scan.go` to see what's blocked. |
| `external_fetch host wildcard wildcard middle position` | Wildcard `*` is not the leftmost label, or you have more than one `*`. | Use `*.tmdb.org` or `api.tmdb.org`, not `cdn.*.tmdb.org`. |
| `installer: slot "admin.section" not allowed` | Refonte 2-kinds bans plugin frontends in privileged areas. | Pick a non-critical slot from the whitelist or rethink the plugin. |
| `cosign verify-blob: failed signature` | Public key on the server doesn't match the private key used to sign. | Re-sign with the right key, or update `EVEY_PLUGINS_COSIGN_PUBLIC_KEY`. |
| Plugin starts then dies with exit 137 | OOM killed by cgroup. | Lower memory usage or increase `EVEY_PLUGINS_MEMORY_MB`. |
| Frontend module not visible | Plugin not enabled, slot mismatch, or `web_root` prefix wrong in `frontend.src`. | Check the admin Plugins page (status badge), confirm the slot string matches a real `<PluginSlot name=...>` mount, look at the network panel for `/api/v1/plugins/{id}/assets/...` calls. |

## 10. Going further

- Security model end-to-end: `docs/plugins.md#security-model`
- Host API surface: `docs/plugins.md#host-api-reference`
- Available slots: `docs/plugins.md#available-plugin-slots`
- SDK code per language: `sdk/`
- Reference plugins: `examples/plugins/` (TMDB, Discord, Spotify, weather, library-stats)
- Reference themes: `examples/themes/` (dark-cozy, aurora, minimal-player)
