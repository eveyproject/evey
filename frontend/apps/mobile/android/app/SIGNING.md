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

## IMPORTANT: rotate the signing key

The previous `release.keystore` and its passwords (`evey2026`) were committed to this
public git history. Even though the file has now been removed from tracking, **the
history still contains it** and the passwords are compromised. Rotating the upload key
is strongly recommended before the next Play Store release (this changes the app's
signing identity, so it must be a deliberate, planned operation, not done automatically
here).
