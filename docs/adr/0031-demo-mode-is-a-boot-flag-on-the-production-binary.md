# ADR-0031: The demo is the production binary behind one boot flag

- Status: Accepted
- Date: 2026-09-28
- Scope: the public demo instance (`EVEY_DEMO=1`). Design history in
  [docs/design/demo-mode.md](../design/demo-mode.md).

## Context

A public demo shows the admin panel, and on a demo every visitor is an
administrator. A separate demo build would drift from the product the day a
feature landed on one side only; `if demo` in the handlers would drift the day
a handler forgot. The demo must run exactly what Docker Hub publishes and still
refuse what a stranger must not do to the host.

## Decision

- **One binary, one image, one compose file.** `EVEY_DEMO=1` is read once at
  boot into `cfg.Server.Demo`, never persisted, never shown by the panel. It is
  the one deliberate exception to "operator settings live in the panel": a
  switch a visitor could flip is not a switch.
- **Deny by default, at two points and nowhere else.** `internal/pkg/demo` is
  the policy: the admin RPCs that stay open, the ones locked, the user RPCs
  locked because the account is shared (`UpdateUser` would change the shared
  password), and the HTTP patterns on both sides. The gRPC `DemoInterceptor`
  (last in the chain, after auth) and the HTTP `DemoLockMiddleware` (which asks
  the mux for the resolved pattern) enforce it. An admin operation in neither
  list is locked.
- **The list cannot rot.** `internal/contractguard/demo_policy_test.go` refuses
  the build when an admin RPC, an admin-gated route or a state-changing route
  is unclassified, and when an entry names something that no longer exists.
  Every entry carries its reason.
- **The server refuses, the clients grey out.** Every refusal is
  `errors.demo.locked`, translated - never a bare 403. The public setup-status
  and `/healthz` say `demoMode`; the settings GET reports each entry `locked`.
  The web greys its write controls from that flag (per control where reads and
  writes share a page, a disabled fieldset on operator-only pages); mobile and
  TV show the banner and the credentials on the login screen.
- **One account, many visitors at once.** `demo` is shared by everyone signed
  in, so a preference written on the account or its profile is a preference
  changed under everyone: those writes (profile locale, region, theme,
  avatar, languages, home layout, notification settings, parental PIN, clear
  history) are locked, and the clients keep the visitor's own choice in the
  browser (locale and theme stores, a local home layout). The shared stores
  ignore what the shared profile says on the demo.
- **The demo is self-sufficient, in both directions.** With the flag on, the
  server bootstraps itself at every boot (`internal/app/demo`, from the
  product's own services, idempotent): the `demo` administrator, a few
  accounts that each show one product rule - a household with a kids profile
  behind a PIN, a viewer with a device cap and a bitrate ceiling, a guest
  confined to the films, a disabled account - the four libraries under
  `EVEY_DEMO_MEDIA_DIR`, and the freely licensed media of the embedded
  manifest (`internal/pkg/demo/manifest.go`: Blender open movies up to 4K
  60 fps with 5.1 and multi-language subtitles, a public-domain 1954 series,
  CC-BY music, photos), each library scanned as its files land. The
  bootstrap leaves a marker in the server config; a boot WITHOUT the flag
  that finds the marker tears everything down - libraries, accounts, the
  manifest's files, the marker - and touches nothing else. When `demo` is the
  last administrator it is kept with a random password rather than deleted
  (deleting it would reopen the setup wizard on a live server). `evey admin
  demo-seed` runs the same bootstrap by hand. The plugin panel is shown the
  same way: the bootstrap installs the demo plugins (`demo.Plugins`, the
  weather example on Paris) from the bundles the operator drops under
  `EVEY_DEMO_PLUGIN_DIR`, enables them and re-applies their config at every
  boot; a missing bundle is skipped. Plugins are visual on the demo: the
  catalogue, the config and the logs read, every write - install, enable,
  config, limits - locked, since the seeded plugin is shared by every visitor. `ImpersonateUser` stays open:
  the minted token is judged by this same policy, as a non-administrator on
  top of it. API-key creation stays open too, so the API can be tried in read
  mode; a key can do nothing the visitor cannot.
- **What the open reads say about the other visitors is blanked.** A generic
  `RedactInterceptor` clears the proto fields named in `demo.RedactedFields`
  (`client_ip`, `ip_address`, `user_agent`) in every response, so the sessions
  page, the presence list and the login history show devices and profiles,
  never addresses. By field name, not by RPC: a new response carrying an
  address is covered the day it ships.
- **The web signs demo visitors in by itself.** The hourly restore wipes every
  refresh token, so a visitor's session dies mid-visit; the credentials being
  public, the login route signs in as `demo` on its own when the server says
  it is the demo, and holds the form open for a day after a deliberate
  sign-out (`?manual=1` always shows it). A longer cookie would not do: the
  server no longer knows the token after the restore.
- **Outbound ports are sinks at composition**, not refused at a handler: no
  emailer is registered, the webhook sender is `Discard`, no push sender is
  built. Scheduled tasks and event subscribers are covered the same way.
- **Reset is the host's job**: `scripts/demo-restore.sh` restores a seeded
  data directory when nobody is watching (the dashboard's active-stream count,
  read as the demo account), at most hourly, and once a day regardless.
  Not on logout: the account is shared. No reset endpoint, no demo migration.

## Consequences

- The demo cannot drift from the product because it is the product. The demo
  artefacts are one flag, one policy file, one guard, one interceptor, one
  middleware, one catalogue key and a host script.
- Adding an endpoint now costs one line in the policy, paid by a failing build
  at the moment the endpoint is written.
- Secret masking (`IsSecretKey`, plugin config, log redaction) is a property of
  the store, shipped before this mode and required by it: a read-only panel
  that serves its keys is worse than no demo.
- Credentials are `demo` / `demo`, and the demo exercises the real login path.
  The production password floor is eight characters; the demo composes the
  user service with `WithMinPasswordLength(4)` (wire, not a mode the service
  knows about) so the setup wizard can seed that account. The web wizard
  mirrors the floor it will be judged by.
- The demo tracks the last release, not `master`: visitors judge what ships.
