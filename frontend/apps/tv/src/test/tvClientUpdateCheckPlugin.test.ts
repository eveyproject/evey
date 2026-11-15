import { createRequire } from "node:module";
import { describe, it, expect } from "vitest";

// The TV's Android project is a prebuild artifact, so the Gradle hook that
// runs the client-update check before any build is written by a plugin. Pin
// that it lands in app/build.gradle, once.
const require_ = createRequire(import.meta.url);
const configPluginsPath = require_.resolve("expo/config-plugins");
type ManifestConfig = { modResults: { manifest: { application: { "meta-data"?: { $: Record<string, string> }[] }[] } } };
let manifestMod: ((cfg: ManifestConfig) => ManifestConfig) | undefined;
let gradleMod: ((cfg: { modResults: { contents: string } }) => { modResults: { contents: string } }) | undefined;
require_.cache[configPluginsPath] = {
  id: configPluginsPath,
  filename: configPluginsPath,
  loaded: true,
  exports: {
    withAppBuildGradle: (config: unknown, fn: typeof gradleMod) => ((gradleMod = fn), config),
    withAndroidManifest: (config: unknown, fn: typeof manifestMod) => ((manifestMod = fn), config),
  },
} as unknown as NodeJS.Module;
const plugin = require_("../../plugins/withClientUpdateCheck.js") as ((c: unknown) => unknown) & { MARK: string; META: Record<string, string> };

describe("withClientUpdateCheck", () => {
  it("makes every build depend on the client-update check", () => {
    plugin({});
    const out = gradleMod!({ modResults: { contents: "android {\n}\n" } }).modResults.contents;

    expect(out).toContain('commandLine "node", "../../../scripts/check-expo-updates-patch.mjs", "--app", "tv"');
    expect(out).toContain('preBuild.dependsOn("checkClientUpdates")');
  });

  it("records the release the build is made from, and fails without one", () => {
    plugin({});
    const out = gradleMod!({ modResults: { contents: "android {\n}\n" } }).modResults.contents;

    expect(out).toContain('commandLine "git", "log", "-1", "--format=%ct"');
    expect(out).toContain('android.defaultConfig.resValue "string", "evey_client_update_floor", "${clientUpdateFloor}000"');
    expect(out).toContain("throw new GradleException(");
  });

  it("verifies through the certificate sent with an update and keeps one set of updates, written once", () => {
    plugin({});
    const chain = "expo.modules.updates.CODE_SIGNING_INCLUDE_MANIFEST_RESPONSE_CERTIFICATE_CHAIN";
    const cfg: ManifestConfig = { modResults: { manifest: { application: [{ "meta-data": [{ $: { "android:name": chain, "android:value": "false" } }] }] } } };
    const entries = manifestMod!(manifestMod!(cfg)).modResults.manifest.application[0]!["meta-data"]!;

    expect(entries).toEqual([
      { $: { "android:name": chain, "android:value": "true" } },
      { $: { "android:name": "expo.modules.updates.EXPO_SCOPE_KEY", "android:value": "evey" } },
    ]);
  });

  it("writes the block once, even when prebuild runs twice", () => {
    plugin({});
    const once = gradleMod!({ modResults: { contents: "android {\n}\n" } }).modResults.contents;
    const twice = gradleMod!({ modResults: { contents: once } }).modResults.contents;

    expect(twice).toBe(once);
    expect(twice.split(plugin.MARK).length).toBe(2);
  });
});
