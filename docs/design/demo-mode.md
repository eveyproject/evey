# Demo mode - design proposal

Date: 2026-09-08
Status: **Proposal. Nothing here is implemented.**

> This is a design document, not an architecture decision record. It describes a
> mode the server does not have yet, so it records an intention rather than a
> decision the code embodies. It becomes an ADR - a short one - the day the mode
> ships, and only for the parts that turned out to be irreversible.

## Context

A public demo instance is the shortest answer to "what does it actually look
like". Nothing in Evey supports one today: the server assumes its operator is
its owner, and the first account created is an administrator with full control
over the machine's media, plugins and mail configuration. Handing that to the
internet is not an option.

Two obvious shapes are both wrong. A separate demo build - a branch, a build
tag, a forked compose file - answers the security question and creates a worse
one: it is a second code base in everything but name, it drifts the moment a
feature lands on one side only, and the drift is invisible because the demo
keeps working. It just stops being the product. Scattering `if demo` through the
handlers keeps one build and loses the guarantee, because the next handler
forgets and the way you find out is a stranger deleting a library.

A third constraint rules out the project's usual answer. Operator settings
belong in the admin panel, never in an environment variable. Demo mode inverts
that rule: on a demo every visitor reaches the admin panel, so a setting the
panel can write is a setting a visitor can turn off. The switch has to sit where
the running application cannot reach it.

### What Jellyfin does, measured

Their demo signs in as `demo` with no password, and the instance is restored at
the top of every hour. Reading the policy their API returns for that account:

| Policy flag | Value |
| --- | --- |
| `IsAdministrator` | false |
| `EnableContentDeletion` | false |
| `EnableCollectionManagement` | false |
| `EnableSubtitleManagement` | false |
| `EnableUserPreferenceAccess` | true |

So Jellyfin has no demo mode. It has an ordinary non-administrator account and
an hourly restore. There is nothing to lock because nothing dangerous is
reachable, and the restore only has to undo a changed password, a preference and
some watch state. That is why it looks effortless.

The demo we want is a strictly harder problem: it shows the admin panel. That
choice is what creates the work, and it is worth being explicit that we are not
copying Jellyfin here - we are taking on the thing they avoided, because an
Evey demo that hides the admin panel hides most of the product.

### What the authorization model actually is

The obvious idea - add a `demo` role with narrow rights - does not survive
contact with the code. There are no roles. `Role` has exactly two values
(`interceptors/rbac.go:4-9`), the claim that carries it is a boolean
(`internal/pkg/auth/jwt.go:46-49`), the column is `is_admin BOOLEAN`
(`migrations/000001_init.up.sql:8`), and a guard already asserts that every
entry in `MethodRoles` is `RoleAdmin` (`rbac_test.go:528-536`).

More decisive than the missing machinery: the axis we need does not exist. A
demo account is not a rung below administrator, it is "administrator minus the
writes" - a second, orthogonal classification. Adding a third value to a
one-dimensional scale does not express it, so a `demo` role would cost a token
claim, a migration, eight token-minting sites and a broken guard, and would
still leave every one of the 195 per-operation decisions to make by hand.

What does exist, and is worth more: a single interception point covering both
transports. The grpc-gateway calls the gRPC listener back over loopback
(`cmd/evey/gateway.go:22-23`), so every transcoded route passes the same
interceptor. And the repository already has the doctrine that keeps a list from
rotting - `TestAllRegisteredMethodsAreClassified` (`rbac_test.go:471-525`)
fails the build on any RPC that is not classified.

### The half of the problem that has nothing to do with writes

The admin panel, read-only, hands out secrets today. `ListServerConfigEntries`
(`server_config_handler.go:56-69`) is a generic key/value store with no notion
of a secret, and the panel fetches it once and spreads it across every card. It
returns, in clear text: the TMDB, OMDb, OpenSubtitles and IntroDB keys, the SMTP
password, the VAPID private key (which is enough to forge signed push messages
to every subscriber), and raw share tokens, each of which is an unauthenticated
download URL. The admin log routes (`admin_handler.go:76-83`) serve the server's
logs with no redaction at all.

No write lock fixes any of that. A demo that locks every button and still serves
that payload is worse than no demo.

The masking exists already but is not wired: `maskSecret`
(`webhook_handler.go:113-119`) is used for webhook secrets only, and
`Plugin.PublicConfig()` (`internal/domain/plugin/model.go:84-120`) is the one
declarative notion of a secret field in the repository - applied on the public
plugin route, never on the admin one.

## Decision

1. **One binary, one image, one compose file.** The demo runs exactly what is
   published to Docker Hub. No demo branch, no build tag, no `//go:build demo`.
   A change the demo needs ships to everyone.

2. **A boot flag, resolved once, immutable.** `EVEY_DEMO=1` is read at startup
   into an immutable config value. It is never persisted, never writable, and
   the admin panel neither shows nor edits it. This is the narrow bootstrap
   category the "no environment settings" rule already allows, and it is here
   for the reason that rule anticipates: a switch a compromised administrator
   can flip is not a switch.

3. **Deny by default: every admin operation is refused except a named list of
   reads.** Not a deny-list of dangerous operations - an allow-list of harmless
   ones. The difference is what happens to the operation nobody classified: with
   a deny-list it ships open, with an allow-list it ships closed. This is the
   same shape as ADR-0012 for plugin permissions.

   The list has to be written by hand, RPC by RPC, because the names lie. Of the
   137 admin RPCs, 46 read like reads and at least eight of those write or
   disclose: `PreviewPluginUpdate` downloads a caller-supplied URL
   (`internal/app/plugin/preview.go:110`), `ScanMusicDirectory` scans an
   arbitrary absolute path (`music_handler.go:331-360`), `DetectHardware` spawns
   ffmpeg, `ListShareLinks` returns live tokens, and `GET .../sync-preview`
   transcodes on every call. Classifying by verb or by name prefix would ship
   all eight open.

4. **The server refuses; the client greys out. Never the reverse.** A
   greyed-out input is a courtesy, and a courtesy is not a control: every locked
   operation must fail on the server when called directly with curl. This is not
   hypothetical here - `TokenRedactMiddleware` accepts `?token=` on every route
   including admin ones (`http/helpers.go:208-226`), so with a published demo
   password anyone can hand-craft an admin call in a URL bar. A refusal returns
   a translated, explicit message ("not available on the demo"), never a bare
   403 - a visitor has to be able to tell a demo boundary from a bug.

5. **Secrets are masked before the demo ships, and masking is a property of the
   config store, not of demo mode.** A `IsSecretKey` predicate in the config
   layer, applied in `server_config_handler.go`, `admin_plugin.go:285-292` and
   `share_handler.go:98`, with write-only semantics on `Set` so saving a form
   does not persist the mask. This is worth doing whether or not the demo
   happens: a VAPID private key in a JSON response is not something an
   administrator needs to read back.

6. **The lock list is published, so no client ever hard-codes it.** The settings
   endpoint already returns one entry per setting, each with its own key; each
   entry gains `locked: true`. Operation-level locks are listed the same way in
   the server-info payload. The three apps grey out what the server says is
   locked and nothing else. Without this, web, mobile and TV each carry their
   own copy of the list and they diverge on the first new setting - the same
   drift this ADR exists to prevent, moved into the UI.

7. **A static guard keeps the list honest.** A `contractguard` test asserts that
   every admin RPC and every state-changing route is classified: allowed on the
   demo, or locked. A new endpoint that is neither fails the build. This is what
   makes the single code base hold - without it the deny-list becomes a snapshot
   of the server on the day it was written, and rots silently. The same doctrine
   already governs admin UI surfaces
   (`TestEveryWebSurfaceReachingAnAdminRPCIsClassified`).

8. **Dangerous ports are neutralised at composition, not merely refused.** Under
   the flag the SMTP, webhook and push senders are wired to no-op sinks, and the
   plugin installer refuses remote sources. Refusing at the handler protects the
   endpoint; replacing the adapter also protects every other road to it - a
   scheduled task, an event subscriber, a plugin.

9. **Reset is not a feature of Evey.** The host restores a seeded database dump
   and data directory on a schedule: stop, restore, start. No reset endpoint, no
   self-wiping timer, no demo-only migration. None of that would ever run in
   production, which is precisely what makes it the code that breaks.

10. **The demo says it is one.** The public server-info payload gains a boolean
   and the three clients render a banner from one translation key.

## What is locked, what is not

**Locked** (server refuses, UI greys out):

- Plugins: install, update, uninstall, and installation from a URL. The
  catalogue stays browsable and the installed plugin's own configuration stays
  editable - that is the part worth showing.
- Network and addressing: published URL, remote access, trusted proxies, ports,
  CORS.
- Identity and mail: SMTP, OIDC, password reset, user creation and deletion.
- Storage and media: library creation and deletion, filesystem browsing, media
  file repair and every in-place write, backup restore.
- The machine: server restart and shutdown, log download, cache path changes.

**Open**, because it is the product: browsing, playback with real transcoding,
search, profiles, parental control, playlists, collections, favourites,
subtitles, theme and language, SyncPlay, the admin panel in read mode, and the
weather plugin's configuration.

**Pre-installed rather than installable:** the `weather` example plugin ships in
the seeded data directory. It is the cheapest honest demonstration of the plugin
system - it renders a real home-page section, it fetches from a real third party
through the egress broker, and its two configuration fields are safe to expose
because the manifest pins the hosts it may reach.

## Reset cadence

**When nobody is watching, at most once an hour.** The host asks the server
whether any playback session is active; if none is, it restores. That is a
`curl` and an `if` in the restore script, no application code, and it lands
close to what "reset when the visitor leaves" was reaching for without any of
its failure modes.

**Not on logout, and this is a property of the design rather than a cost
question.** The account is shared, so several visitors are signed in as `demo`
at once. A logout-triggered restore wipes the session of everyone else still
watching, and the common case - closing the tab - never triggers it at all. It
fires when it should not and stays silent when it should fire.

A per-visitor ephemeral profile would make logout meaningful, and it is
deliberately rejected: it is the one piece of demo-only business logic in an ADR
whose whole point is that the demo runs the same code as production. The
idle-triggered restore gets the same result with a shell script.

**A daily floor.** Even with the idle trigger, the instance is restored once a
day regardless, so a demo that is busy for eighteen hours straight does not
accumulate a day of edits.

**Credentials: `demo` / `demo`.** A visible password rather than Jellyfin's
empty one, so the demo exercises the real login path instead of a special case.
Note that this password is published, and `?token=` is accepted on every route
(`http/helpers.go:208-226`): every lock has to hold against a hand-written
request, not merely against a disabled button.

## Consequences

- The demo cannot drift from the product, because it is the product. The
  demo-specific artefacts are one flag, one allow-list, one guard test, one
  banner, and a host-side restore script.
- Roughly ten server files, none touched deeply: the interceptor and its HTTP
  sibling, the boot flag, the guard, the `locked` field on the existing settings
  definitions, and the four files that mask secrets. Compare with the demo-role
  option, which needs a migration, a token claim, eight minting sites and a
  broken guard before a single operation is classified.
- The allow-list is a standing maintenance obligation, paid at the moment a new
  endpoint is added, by a failing build. That is the cheapest moment.
- The HTTP half is the fragile half. Fifty-eight admin routes, no central table,
  and `http.ServeMux` does not expose its patterns at runtime, so the guard has
  to parse the source - as `internal/contractguard/http_access_options_test.go`
  already does. Five of those routes hide their middleware inside a handler
  constructor (`server.go:593-597`), so a guard that only reads `server.go`
  would call them unguarded.
- Showing a degraded admin panel is more work than hiding it, and it is the
  reason this ADR is long. The alternative - a plain non-administrator account,
  the Jellyfin shape - remains available and would cost almost nothing.
- The demo instance is the first publicly reachable Evey, so it is the first
  target. It shares no secret, no key and no network with any other instance,
  and holds nothing worth stealing: seeded accounts and freely licensed media.
- Seeded content must be redistributable. Blender Foundation films (CC-BY) have
  real TMDB entries, so artwork and metadata exercise the real enrichment path
  rather than a fixture.

## Open question for the operator

**Does the demo track `master` or the last release?** Tracking a release is what
visitors should be judging. Tracking `master` turns the demo into a staging
environment, which is a different job for a different audience.
