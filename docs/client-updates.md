# App updates served by the instance

The phone and TV apps load their JavaScript from the Evey instance they are
connected to. A fix to the app's JavaScript then reaches a device without a
store release, as soon as its server is upgraded. Native changes still need
one.

An app only ever moves forward: it takes a release newer than what it runs
and keeps its own code in front of an older server.

## How it fits together

- **The image ships the bundles.** `make client-updates` builds one signed
  bundle per app into `client-updates/`, and the image copies that directory
  to `/app/client-updates` (`EVEY_CLIENT_UPDATES_DIR`). An image built without
  that step serves no update and the apps keep the code they were built with.
- **The server only hands bytes out.** `GET /api/v1/client-updates/{app}/manifest`
  and `.../assets/{platform}/{runtime}/{name}` speak the Expo Updates protocol
  (`internal/adapters/driving/http/client_update_handler.go`). The instance
  holds no key and builds no manifest, and only opens files inside that
  directory.
- **The apps verify.** Every bundle is signed by the project and sent with
  the certificate of the key that signed it; the apps check that certificate
  against the root compiled into them, `frontend/client-updates/certificate.pem`.
  An instance can only distribute published code; anything else is refused on
  the device.
- **The apps never go back.** See "Release floor" below.
- **The apps follow the active server.** Once a server is chosen the app points
  expo-updates at it and downloads its bundle in the background
  (`frontend/packages/client/src/native/clientUpdates.ts`). The bundle starts
  with the next launch; a restart is offered only when it is another app
  version than the one running, and never over the player. With no server
  chosen the app asks nobody.

## Native runtime version

A bundle only runs on the native build it was made for. Each app declares a
`runtimeVersion` in its `app.json` (`mobile-1`, `tv-1`), and a bundle is stored
and requested under that name.

**Bump `runtimeVersion` in the same change as anything native**: a new or
upgraded native module, an Expo SDK or React Native upgrade, a config plugin,
the Android manifest, or the Go code bound into `offline-audio`. The phone
keeps its Android project in git, so the value also lives in
`frontend/apps/mobile/android/app/src/main/res/values/strings.xml`.

An instance has no bundle for a runtime it was not built with: an older or
newer store build simply keeps its own code.

## Release floor

A signature says who published a release, not that it is recent. Without more,
any server, or anyone on the path of a plain HTTP connection, could hand an app
an old signed release and bring back a bug fixed since.

- A release is dated by the commit it was built from (`createdAt` in the
  signed manifest, committer time).
- Each device keeps a floor: the newer of the commit its store build was made
  from (recorded by Gradle) and the newest served release that rendered on it.
  The floor only goes up.
- The app downloads nothing below the floor and starts the newest release it
  holds at or above it, whichever server it came from. A server that serves
  nothing, or something older, leaves the app on what it has.
- The code built into the app is always allowed to run. It is what starts when
  nothing stored qualifies. It is dated by its commit too, not by the moment
  the app was compiled, so the release of the same commit, and of any later
  one, is taken over it.
- The JavaScript keeps the same floor on its side and refuses to run below it.

What follows from it:

- Facing an older server, an app runs newer JavaScript than that server
  shipped, like a store build does. The server API has to stay compatible with
  newer clients.
- When the guard drops a release that had already rendered, the one before it
  is usually below the floor: the app runs its built-in code until the instance
  publishes a newer release.
- A build whose floor cannot be read accepts no update at all.
- Two builds of one commit are two releases with nearly the same date. The
  apps store one release per date: should both dates collide, the second is
  not stored and the app keeps what it runs. Build once per release.

Nothing unsigned steers any of this: the apps ignore manifest filters,
server-defined headers, the extensions part and directives, and send servers
neither an installation id nor the text of a previous crash.

## When an update misbehaves

- A bundle that crashes before its first render is dropped by expo-updates,
  which falls back to the previous one or to the code built into the app.
- A bundle that renders and then dies at every launch is caught by the app's
  own guard (`clientUpdateBootGuard`): after three launches in a row that
  ended without settling, the next one asks for the same fallback.
- A dropped bundle is not tried again. The instance must publish another one.

## The expo-updates patch

`frontend/patches/expo-updates@*.patch` carries what the design needs and
expo-updates has no setting for:

- asset addresses in a manifest may be relative, resolved against the manifest
  address, so one signed manifest is valid on every instance;
- the guard's rollback request drops an update even when it has started
  successfully before;
- the release floor, and launching by release instead of by server address;
- the code built into the app is dated by its commit;
- a release already stored is adopted, not downloaded again, when another
  address serves it (the same instance from the local network and from
  outside);
- every asset must carry its hash in the signed manifest;
- certificates are checked at the later of the device clock and the floor, and
  a release cannot be dated after the expiry of its signing certificate;
- the unsigned inputs and identifying headers listed above are dropped.

Re-create the patch when expo-updates is upgraded; the dependency is pinned to
the exact patched version.

## What a build must have

`frontend/scripts/check-expo-updates-patch.mjs` checks, for both apps: that
the patch is in the sources Android compiles, that the certificate and its
metadata are configured (without a certificate expo-updates verifies nothing),
that the certificate is a root and the apps verify through it, that the build
records its release floor, that no setting widens what the apps accept, and
that the native runtime version matches `app.json`. It runs before every Gradle build of the apps, and
in the gate on any branch that touches the dependencies, their patches, the
certificate or an app's native side.

## Keys

Two keys, made by `frontend/scripts/client-update-certificate.sh`:

- **The root** (`root <dir>`), once. Its certificate is
  `frontend/client-updates/certificate.pem`, compiled into every store build;
  its key is encrypted under a passphrase and stays offline, with a backup.
  It never goes to a build machine. Losing it, or having to replace it, means
  a new store build of both apps before any update can be served again.
- **The signing certificate** (`leaf <dir> [days]`), one year by default,
  issued by the root. Its key is what a release build holds.

Renew the signing certificate before it expires: the publication tool refuses
one with less than 30 days left, and devices refuse a release once its
certificate has expired, so an image stops handing out its bundle a year after
its certificate was issued (devices keep what they already hold). There is no
revocation: a leaked signing key is good until its certificate expires, and
only for releases dated before that. Replacing the root is the only earlier
cut-off.

## Releasing

1. `EVEY_CLIENT_UPDATE_KEY_FILE=<signing key> EVEY_CLIENT_UPDATE_CERT_FILE=<signing certificate> make client-updates`
   once, on the release commit with a clean tree, before the image build. The
   Hermes bytecode is not reproducible: two runs give two different updates,
   so every architecture of a release must ship the same output. The tool
   refuses to publish over a newer release.
2. Build the image. The build fails on a bundle that is unsigned, was built
   from another commit, or is for a runtime the apps no longer declare.

Not wired yet: a release pipeline that signs once and hands the same output to
every architecture's image build.

No private key ever enters the repository or an image, and keys are only ever
given as file paths.
