const { withAndroidManifest, withAppBuildGradle } = require("expo/config-plugins");

// What the TV's generated Android project needs for the updates served by an
// instance (docs/client-updates.md). The phone has the same in its tracked
// build.gradle and manifest; the TV's are written by prebuild, so they live
// here:
// - every Gradle build first runs the client-update check
//   (frontend/scripts/check-expo-updates-patch.mjs);
// - the build records the release it is made from, the oldest served update
//   the app accepts;
// - updates are verified through the certificate sent with them, against the
//   root the app carries;
// - stored updates are one set, whichever server address they came from
//   (expo-updates otherwise keeps one set per address).
// Expo's own config has no key for the last two settings.
const MARK = "// --- evey: client updates (withClientUpdateCheck) ---";
const BLOCK = `${MARK}
tasks.register("checkClientUpdates", Exec) {
    workingDir rootProject.projectDir
    commandLine "node", "../../../scripts/check-expo-updates-patch.mjs", "--app", "tv"
}
preBuild.dependsOn("checkClientUpdates")

def clientUpdateFloor = providers.exec {
    workingDir rootProject.projectDir
    commandLine "git", "log", "-1", "--format=%ct"
}.standardOutput.asText.get().trim()
if (!(clientUpdateFloor ==~ /[0-9]+/)) {
    throw new GradleException("client updates: git gave no commit time, the app would accept no update")
}
android.defaultConfig.resValue "string", "evey_client_update_floor", "\${clientUpdateFloor}000"
`;
const CHAIN = "expo.modules.updates.CODE_SIGNING_INCLUDE_MANIFEST_RESPONSE_CERTIFICATE_CHAIN";
const SCOPE = "expo.modules.updates.EXPO_SCOPE_KEY";
const META = { [CHAIN]: "true", [SCOPE]: "evey" };

function withClientUpdateCheck(config) {
  const withGradle = withAppBuildGradle(config, (cfg) => {
    if (!cfg.modResults.contents.includes(MARK)) {
      cfg.modResults.contents = `${cfg.modResults.contents.replace(/\n*$/, "\n")}\n${BLOCK}`;
    }
    return cfg;
  });
  return withAndroidManifest(withGradle, (cfg) => {
    const application = cfg.modResults.manifest.application[0];
    const metaData = (application["meta-data"] ?? []).filter((entry) => !(entry.$["android:name"] in META));
    for (const [name, value] of Object.entries(META)) metaData.push({ $: { "android:name": name, "android:value": value } });
    application["meta-data"] = metaData;
    return cfg;
  });
}

module.exports = withClientUpdateCheck;
module.exports.MARK = MARK;
module.exports.META = META;
