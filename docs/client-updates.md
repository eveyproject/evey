# App updates served by the instance

The phone and TV apps load their JavaScript from the Evey instance they are
connected to. A client then runs the code that was released with its server,
and a fix to the app's JavaScript reaches a device without a store release.
Native changes still need one.

## How it fits together

- **The image ships the bundles.** `make client-updates` builds one signed
  bundle per app into `client-updates/`, and the image copies that directory
  to `/app/client-updates` (`EVEY_CLIENT_UPDATES_DIR`). An image built without
  that step serves no update and the apps keep the code they were built with.
- **The server only hands bytes out.** `GET /api/v1/client-updates/{app}/manifest`
  and `.../assets/{platform}/{runtime}/{name}` speak the Expo Updates protocol
  (`internal/adapters/driving/http/client_update_handler.go`). The instance
  holds no key and builds no manifest.
- **The apps verify.** Every bundle is signed by the project key and checked
  against `frontend/client-updates/certificate.pem`, which is compiled into the
  apps. An instance can only distribute published code; anything else is
  refused on the device.
- **The apps follow the active server.** Once a server is chosen the app points
  expo-updates at it and downloads its bundle in the background
  (`frontend/packages/client/src/native/clientUpdates.ts`). The bundle starts
  with the next launch; a restart is offered only when it is another app
  version than the one running, and never over the player.

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

## When an update misbehaves

- A bundle that crashes before its first render is dropped by expo-updates,
  which falls back to the previous one or to the code built into the app.
- A bundle that renders and then dies at every launch is caught by the app's
  own guard (`clientUpdateBootGuard`): after three launches in a row that
  ended without settling, the next one asks for the same fallback.
- A dropped bundle is not tried again. The instance must publish another one.

## The expo-updates patch

`frontend/patches/expo-updates@*.patch` carries two changes the design needs:

- asset addresses in a manifest may be relative, resolved against the manifest
  address, so one signed manifest is valid on every instance;
- the guard's rollback request drops an update even when it has started
  successfully before.

Re-create the patch when expo-updates is upgraded. The gate checks that both
changes are in the sources the Android build compiles
(`frontend/scripts/check-expo-updates-patch.mjs`), on any branch that touches
the dependencies, their patches or an app's native side.

## Releasing

1. `EVEY_CLIENT_UPDATE_KEY=<private key> make client-updates` once, before the
   image build. The Hermes bytecode is not reproducible: two runs give two
   different updates, so every architecture of a release must ship the same
   output.
2. Build the image.

The private key never enters the repository or an image.
