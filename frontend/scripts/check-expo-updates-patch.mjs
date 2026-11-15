#!/usr/bin/env node
// What an Android build of the apps must have for the updates served by an
// instance to be safe (docs/client-updates.md). None of it fails a build on
// its own: a missing certificate, for one, makes expo-updates skip signature
// verification entirely and accept whatever a server sends.
//
// Run by the gate when the Android side changes, and by every Gradle build of
// both apps (preBuild), so no APK or AAB is produced from a tree that fails.

import crypto from "node:crypto";
import fs from "node:fs";
import { createRequire } from "node:module";
import path from "node:path";
import { fileURLToPath } from "node:url";

const FRONTEND = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
// The gate checks both apps; a Gradle build passes --app and checks its own,
// so a stale generated TV project does not stop a phone build.
const only = process.argv[2] === "--app" ? process.argv[3] : null;
const APPS = ["mobile", "tv"].filter((app) => !only || app === only);
if (APPS.length === 0) {
  console.error("client updates: --app must be mobile or tv");
  process.exit(1);
}
const METADATA = { keyid: "main", alg: "rsa-v1_5-sha256" };
// Settings that widen what the apps accept.
const FORBIDDEN_CONFIG = ["codeSigningAllowUnsignedManifests", "enableBsdiffPatchSupport"];
const FORBIDDEN_MANIFEST = ["CODE_SIGNING_ALLOW_UNSIGNED_MANIFESTS", "ENABLE_BSDIFF_PATCH_SUPPORT"];
// The apps carry the root and verify the certificate sent with each update.
// Expo's config has no key for this setting, so it is written by hand (phone)
// or by a config plugin (TV).
const CHAIN = "CODE_SIGNING_INCLUDE_MANIFEST_RESPONSE_CERTIFICATE_CHAIN";
const FLOOR_RESOURCE = "evey_client_update_floor";
const TV_PLUGIN = "./plugins/withClientUpdateCheck";

const failures = [];
const expect = (ok, message) => ok || failures.push(message);
const readText = (file) => fs.readFileSync(file, "utf8");
const pem = (text) => text.replace(/\s+/g, "");
const certificateText = readText(path.join(FRONTEND, "client-updates", "certificate.pem"));
const certificate = pem(certificateText);
try {
  const root = new crypto.X509Certificate(certificateText);
  // Anything else and the certificate sent with an update is refused, or the
  // key in the build machine's hands is the one every app trusts for good.
  expect(root.ca && root.checkIssued(root) && root.verify(root.publicKey), "client-updates/certificate.pem must be a self-signed certificate authority (client-update-certificate.sh root)");
} catch {
  expect(false, "client-updates/certificate.pem is not a certificate");
}

// The marker is shared by name only: the Kotlin side must look for the exact
// string the boot guard throws.
const marker = /CLIENT_UPDATE_ROLLBACK_MARKER = "([A-Z_]+)"/.exec(readText(path.join(FRONTEND, "packages/client/src/native/clientUpdates.ts")))?.[1];
expect(Boolean(marker), "CLIENT_UPDATE_ROLLBACK_MARKER not found in packages/client/src/native/clientUpdates.ts");

const patched = Object.keys(JSON.parse(readText(path.join(FRONTEND, "package.json"))).pnpm?.patchedDependencies ?? {}).filter((key) =>
  key.startsWith("expo-updates@"),
);

function checkPatch(packageDir, label) {
  const installed = JSON.parse(readText(path.join(packageDir, "package.json"))).version;
  // An upgrade that leaves the old key behind installs expo-updates unpatched.
  expect(
    patched.length === 1 && patched[0] === `expo-updates@${installed}`,
    `${label}: expo-updates ${installed} is installed but the patch is declared for [${patched.join(", ")}]: re-create frontend/patches/expo-updates@${installed}.patch`,
  );
  const read = (rel) => readText(path.join(packageDir, "android/src/main/java/expo/modules/updates", rel));
  const update = read("manifest/ExpoUpdatesUpdate.kt");
  expect(
    update.includes("private fun resolveAssetUrl(") &&
      update.includes('resolveAssetUrl(launchAsset.getString("url"))') &&
      update.includes('resolveAssetUrl(assetObject.getString("url"))'),
    `${label}: relative asset addresses are not resolved against the manifest address (manifest/ExpoUpdatesUpdate.kt)`,
  );
  const handler = read("errorrecovery/ErrorRecoveryHandler.kt");
  expect(
    Boolean(marker) && handler.includes(`contains("${marker}")`) && handler.includes("delegate.dropLaunchedUpdate()"),
    `${label}: the boot guard's rollback request is not honoured (errorrecovery/ErrorRecoveryHandler.kt)`,
  );
  expect(read("errorrecovery/ErrorRecoveryDelegate.kt").includes("fun dropLaunchedUpdate()"), `${label}: ErrorRecoveryDelegate.dropLaunchedUpdate is missing`);
  const startup = read("procedures/StartupProcedure.kt");
  expect(startup.includes("markUpdateUnlaunchable(launchedUpdate)"), `${label}: StartupProcedure does not drop the launched update`);
  const dao = read("db/dao/UpdateDao.kt");
  expect(dao.includes("SET successful_launch_count = 0, failed_launch_count = failed_launch_count + 1"), `${label}: UpdateDao.markUpdateUnlaunchable is missing`);

  // The release floor: nothing older than what this device already ran.
  let floor = "";
  try {
    floor = read("EveyReleaseFloor.kt");
  } catch {
    floor = "";
  }
  expect(floor.includes(`"${FLOOR_RESOURCE}"`) && floor.includes("return Long.MAX_VALUE"), `${label}: EveyReleaseFloor is missing or no longer fails closed`);
  expect(floor.includes("EmbeddedManifestUtils.isBuiltIn(update)") && floor.includes("!isLeftBehind(update)"), `${label}: the code built into the app is no longer told apart from a served release (EveyReleaseFloor.kt)`);
  expect(read("EnabledUpdatesController.kt").includes("EveyReleaseFloor.init(context)"), `${label}: the release floor is never initialised (EnabledUpdatesController.kt)`);
  expect(
    read("selectionpolicy/LoaderSelectionPolicyFilterAware.kt").includes("if (!EveyReleaseFloor.admits(newUpdate))"),
    `${label}: updates below the release floor are still downloaded (LoaderSelectionPolicyFilterAware.kt)`,
  );
  expect(read("procedures/CheckForUpdateProcedure.kt").includes("launchedUpdate == null && !EveyReleaseFloor.admits("), `${label}: an update below the floor is announced as available (CheckForUpdateProcedure.kt)`);
  const launcher = read("selectionpolicy/LauncherSelectionPolicyFilterAware.kt");
  expect(
    launcher.includes("EveyReleaseFloor.admitsForLaunch(it)") && !launcher.includes("config.updateUrl"),
    `${label}: the launcher must pick the newest release at or above the floor, whatever address served it (LauncherSelectionPolicyFilterAware.kt)`,
  );
  expect(
    read("manifest/EmbeddedManifestUtils.kt").includes('EveyReleaseFloor.builtFrom(context)?.let { put("commitTime", it) }') &&
      read("loader/Loader.kt").includes("deleteOtherUpdatesAt(updateEntity!!)"),
    `${label}: the built-in code is dated by its build, not by its commit: a release of the same commit would never be taken (EmbeddedManifestUtils.kt, Loader.kt)`,
  );
  expect(
    read("procedures/RelaunchProcedure.kt").includes("restartAppWhenNothingStarts && getCurrentLauncher().launchedUpdate != null") &&
      read("EnabledUpdatesController.kt").includes("restartAppWhenNothingStarts = true"),
    `${label}: dropping the running update with nothing left to start ends in a crash instead of a restart on the built-in code (RelaunchProcedure.kt)`,
  );
  expect(startup.includes("EveyReleaseFloor.raiseTo(launchedUpdate)"), `${label}: a release that rendered does not raise the floor (StartupProcedure.kt)`);
  expect(
    dao.includes("SET url = :url, headers = :requestHeaders") && read("loader/Loader.kt").includes("setUpdateSource(existingUpdateEntity"),
    `${label}: a release already stored is not adopted when another address serves it (Loader.kt)`,
  );
  expect(update.includes("without a hash") && update.includes("has no readable launch asset"), `${label}: an asset without a hash is still accepted (manifest/ExpoUpdatesUpdate.kt)`);
  expect(read("codesigning/CertificateChain.kt").includes("checkValidity(EveyReleaseFloor.certificateCheckTime())"), `${label}: certificates are checked against the device clock alone (CertificateChain.kt)`);
  expect(
    read("codesigning/CodeSigningConfiguration.kt").includes("createdAt.after(certificateChain.codeSigningCertificate.notAfter)"),
    `${label}: a release may be dated after its signing certificate expires (CodeSigningConfiguration.kt)`,
  );
  // What a server could otherwise steer or learn without any signature.
  const downloader = read("loader/FileDownloader.kt");
  expect(
    downloader.includes("manifestFiltersRaw = null") && downloader.includes("serverDefinedHeadersRaw = null") && downloader.includes('"extensions" -> Unit'),
    `${label}: unsigned filters, server-defined headers or extensions are read again (FileDownloader.kt)`,
  );
  expect(downloader.includes('throw IOException("Directives are not accepted")'), `${label}: directives are accepted (FileDownloader.kt)`);
  expect(!downloader.includes('"EAS-Client-ID"') && !downloader.includes('"Expo-Fatal-Error"'), `${label}: an installation id or a crash text is sent to servers again (FileDownloader.kt)`);
  return installed;
}

function metaData(manifest, name) {
  const match = new RegExp(`android:name="expo\\.modules\\.updates\\.${name}"\\s+android:value="([^"]*)"`).exec(manifest);
  return match ? match[1].replace(/&#x[0-9a-fA-F]+;/g, (entity) => String.fromCodePoint(parseInt(entity.slice(3, -1), 16))).replace(/&quot;/g, '"') : null;
}

const versions = new Set();
const checkedDirs = new Set();
for (const app of APPS) {
  const appDir = path.join(FRONTEND, "apps", app);
  // Each app resolves its own copy: the TV app builds against react-native-tvos
  // and gets a separate install of the same version.
  const packageDir = fs.realpathSync(path.dirname(createRequire(path.join(appDir, "package.json")).resolve("expo-updates/package.json")));
  if (!checkedDirs.has(packageDir)) {
    checkedDirs.add(packageDir);
    versions.add(checkPatch(packageDir, `${app}'s expo-updates`));
  }

  const expo = JSON.parse(readText(path.join(appDir, "app.json"))).expo;
  const updates = expo.updates ?? {};
  expect(typeof expo.runtimeVersion === "string" && expo.runtimeVersion !== "", `${app}: app.json needs a plain string runtimeVersion`);
  expect(updates.enabled === true, `${app}: app.json updates.enabled must be true`);
  // Without a certificate expo-updates verifies nothing.
  const certPath = typeof updates.codeSigningCertificate === "string" ? path.resolve(appDir, updates.codeSigningCertificate) : "";
  expect(
    certPath !== "" && fs.existsSync(certPath) && pem(readText(certPath)) === certificate,
    `${app}: app.json updates.codeSigningCertificate must point at frontend/client-updates/certificate.pem`,
  );
  expect(
    updates.codeSigningMetadata?.keyid === METADATA.keyid && updates.codeSigningMetadata?.alg === METADATA.alg,
    `${app}: app.json updates.codeSigningMetadata must be ${JSON.stringify(METADATA)}`,
  );
  for (const key of FORBIDDEN_CONFIG) expect(!(key in updates), `${app}: app.json updates.${key} must not be set`);

  if (app === "tv") expect((expo.plugins ?? []).includes(TV_PLUGIN), `tv: app.json plugins must list ${TV_PLUGIN}`);

  // The phone keeps its Android project in git; the TV's is generated by
  // prebuild and only checked once it exists.
  const manifestPath = path.join(appDir, "android/app/src/main/AndroidManifest.xml");
  if (!fs.existsSync(manifestPath)) {
    expect(app !== "mobile", "mobile: android/app/src/main/AndroidManifest.xml is missing");
    continue;
  }
  const manifest = readText(manifestPath);
  expect(metaData(manifest, "ENABLED") === "true", `${app}: AndroidManifest updates ENABLED must be true`);
  expect(pem(metaData(manifest, "CODE_SIGNING_CERTIFICATE") ?? "") === certificate, `${app}: AndroidManifest CODE_SIGNING_CERTIFICATE is not frontend/client-updates/certificate.pem`);
  let metadata = null;
  try {
    metadata = JSON.parse(metaData(manifest, "CODE_SIGNING_METADATA") ?? "null");
  } catch {
    metadata = null;
  }
  expect(metadata?.keyid === METADATA.keyid && metadata?.alg === METADATA.alg, `${app}: AndroidManifest CODE_SIGNING_METADATA must be ${JSON.stringify(METADATA)}`);
  expect(metaData(manifest, CHAIN) === "true", `${app}: AndroidManifest ${CHAIN} must be true`);
  // expo-updates keeps one set of updates per server address unless told
  // otherwise: a server serving nothing would then put the app back on its
  // built-in code.
  expect(metaData(manifest, "EXPO_SCOPE_KEY") === "evey", `${app}: AndroidManifest EXPO_SCOPE_KEY must be "evey"`);
  // Without it the app accepts no update at all.
  expect(readText(path.join(appDir, "android/app/build.gradle")).includes(`resValue "string", "${FLOOR_RESOURCE}"`), `${app}: android/app/build.gradle does not record the release floor (${FLOOR_RESOURCE})`);
  for (const name of FORBIDDEN_MANIFEST) expect(!manifest.includes(`expo.modules.updates.${name}`), `${app}: AndroidManifest must not set ${name}`);
  const strings = readText(path.join(appDir, "android/app/src/main/res/values/strings.xml"));
  const runtime = /<string name="expo_runtime_version">([^<]*)<\/string>/.exec(strings)?.[1];
  expect(runtime === expo.runtimeVersion, `${app}: strings.xml expo_runtime_version is ${JSON.stringify(runtime)}, app.json runtimeVersion is ${JSON.stringify(expo.runtimeVersion)}`);
}

if (failures.length > 0) {
  for (const failure of failures) console.error(`client updates: ${failure}`);
  process.exit(1);
}
console.log(`client updates: expo-updates ${[...versions].join(", ")} patched (${checkedDirs.size} install${checkedDirs.size > 1 ? "s" : ""}), release floor and signing configuration in place`);
