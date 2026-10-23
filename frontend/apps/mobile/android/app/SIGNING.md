# Android release signing

`release.keystore` is **not committed** (see root `.gitignore`, `**/release.keystore`).
`build.gradle` reads the signing credentials from environment variables or from
`gradle.properties` (project-level or `~/.gradle/gradle.properties`, the latter is never
committed and is the recommended place for a local/dev machine):

- `EVEY_UPLOAD_STORE_FILE` (optional, defaults to `release.keystore`, relative to
  `android/app/`)
- `EVEY_UPLOAD_STORE_PASSWORD` (required)
- `EVEY_UPLOAD_KEY_ALIAS` (optional, defaults to `evey`)
- `EVEY_UPLOAD_KEY_PASSWORD` (required)

If `EVEY_UPLOAD_STORE_PASSWORD` / `EVEY_UPLOAD_KEY_PASSWORD` are not set, any Gradle task
that builds a release variant (`assembleRelease`, `bundleRelease`, ...) fails immediately
with a clear error instead of silently signing with a fallback secret.

The upload key is the app's signing identity: keep the keystore and its passwords out of
the repository, out of CI logs and out of screenshots. Changing it changes the identity
Google Play knows, so a rotation is a planned operation, never a side effect of a build.
