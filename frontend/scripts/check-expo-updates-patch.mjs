#!/usr/bin/env node
// The apps rely on two changes patched into expo-updates (docs/client-updates.md).
// Without them an Android build still compiles and starts, and then: no update
// ever downloads (asset addresses stay relative), and an update that crashes
// at every launch is never dropped. This reads what the build would compile.

import fs from "node:fs";
import { createRequire } from "node:module";
import path from "node:path";
import { fileURLToPath } from "node:url";

const FRONTEND = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const require = createRequire(path.join(FRONTEND, "apps", "mobile", "package.json"));
const packageDir = path.dirname(require.resolve("expo-updates/package.json"));
const sources = path.join(packageDir, "android/src/main/java/expo/modules/updates");
const read = (rel) => fs.readFileSync(path.join(sources, rel), "utf8");
const failures = [];
const expect = (ok, message) => ok || failures.push(message);

const installed = JSON.parse(fs.readFileSync(path.join(packageDir, "package.json"), "utf8")).version;
const patched = Object.keys(JSON.parse(fs.readFileSync(path.join(FRONTEND, "package.json"), "utf8")).pnpm?.patchedDependencies ?? {}).filter((key) =>
  key.startsWith("expo-updates@"),
);
// An upgrade that leaves the old key behind installs expo-updates unpatched.
expect(
  patched.length === 1 && patched[0] === `expo-updates@${installed}`,
  `expo-updates ${installed} is installed but the patch is declared for [${patched.join(", ")}]: re-create frontend/patches/expo-updates@${installed}.patch`,
);

const update = read("manifest/ExpoUpdatesUpdate.kt");
expect(
  update.includes("private fun resolveAssetUrl(") &&
    update.includes('resolveAssetUrl(launchAsset.getString("url"))') &&
    update.includes('resolveAssetUrl(assetObject.getString("url"))'),
  "relative asset addresses are not resolved against the manifest address (manifest/ExpoUpdatesUpdate.kt)",
);

// The marker is shared by name only: the Kotlin side must look for the exact
// string the boot guard throws.
const guard = fs.readFileSync(path.join(FRONTEND, "packages/client/src/native/clientUpdates.ts"), "utf8");
const marker = /CLIENT_UPDATE_ROLLBACK_MARKER = "([A-Z_]+)"/.exec(guard)?.[1];
expect(Boolean(marker), "CLIENT_UPDATE_ROLLBACK_MARKER not found in packages/client/src/native/clientUpdates.ts");
const handler = read("errorrecovery/ErrorRecoveryHandler.kt");
expect(
  Boolean(marker) && handler.includes(`contains("${marker}")`) && handler.includes("delegate.dropLaunchedUpdate()"),
  "the boot guard's rollback request is not honoured (errorrecovery/ErrorRecoveryHandler.kt)",
);
expect(read("errorrecovery/ErrorRecoveryDelegate.kt").includes("fun dropLaunchedUpdate()"), "ErrorRecoveryDelegate.dropLaunchedUpdate is missing");
expect(read("procedures/StartupProcedure.kt").includes("markUpdateUnlaunchable(launchedUpdate)"), "StartupProcedure does not drop the launched update");
expect(
  read("db/dao/UpdateDao.kt").includes("SET successful_launch_count = 0, failed_launch_count = failed_launch_count + 1"),
  "UpdateDao.markUpdateUnlaunchable is missing",
);

if (failures.length > 0) {
  for (const failure of failures) console.error(`expo-updates patch: ${failure}`);
  process.exit(1);
}
console.log(`expo-updates ${installed}: patch applied`);
