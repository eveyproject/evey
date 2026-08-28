const { withAppBuildGradle } = require("expo/config-plugins");

// Prebuild regenerates android/ with the RN template's debug-signed release
// build (frontend/apps/tv/android is gitignored), which Play refuses. This
// plugin rewrites app/build.gradle so releases are signed with the same
// keystore as the mobile app, read from EVEY_UPLOAD_* env vars or
// ~/.gradle/gradle.properties - see frontend/apps/mobile/android/app/SIGNING.md.
// No hardcoded fallback: a release build without real credentials must fail
// loudly rather than sign with the public debug key.

const SIGNING_DEFS = `
def releaseStoreFileName = System.getenv("EVEY_UPLOAD_STORE_FILE") ?: findProperty('EVEY_UPLOAD_STORE_FILE') ?: 'release.keystore'
def releaseKeyAlias = System.getenv("EVEY_UPLOAD_KEY_ALIAS") ?: findProperty('EVEY_UPLOAD_KEY_ALIAS') ?: 'evey'
def releaseStorePassword = System.getenv("EVEY_UPLOAD_STORE_PASSWORD") ?: findProperty('EVEY_UPLOAD_STORE_PASSWORD')
def releaseKeyPassword = System.getenv("EVEY_UPLOAD_KEY_PASSWORD") ?: findProperty('EVEY_UPLOAD_KEY_PASSWORD')
def hasReleaseSigning = releaseStorePassword && releaseKeyPassword

android {`;

const RELEASE_SIGNING_CONFIG = `        release {
            if (hasReleaseSigning) {
                storeFile file(releaseStoreFileName)
                storePassword releaseStorePassword
                keyAlias releaseKeyAlias
                keyPassword releaseKeyPassword
            }
        }
    }`;

module.exports = function withReleaseSigning(config) {
  return withAppBuildGradle(config, (cfg) => {
    let gradle = cfg.modResults.contents;
    if (gradle.includes("hasReleaseSigning")) return cfg; // already applied

    gradle = gradle.replace(/\nandroid \{/, `\n${SIGNING_DEFS}`);

    // Append a release entry to signingConfigs (the template only has debug).
    gradle = gradle.replace(
      /(signingConfigs \{[\s\S]*?debug \{[\s\S]*?\n        \}\n)(    \})/,
      `$1${RELEASE_SIGNING_CONFIG}`,
    );

    // Point the release build type at it instead of the debug key.
    gradle = gradle.replace(
      /(release \{\n(?:\s*\/\/[^\n]*\n)*\s*signingConfig )signingConfigs\.debug/,
      "$1signingConfigs.release",
    );

    if (!gradle.includes("signingConfigs.release")) {
      throw new Error("withReleaseSigning: failed to rewrite app/build.gradle release signing");
    }
    cfg.modResults.contents = gradle;
    return cfg;
  });
};
