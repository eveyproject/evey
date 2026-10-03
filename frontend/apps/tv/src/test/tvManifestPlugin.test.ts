import { createRequire } from "node:module";
import { describe, it, expect } from "vitest";

// The plugins edit the generated Android manifest at prebuild. A permission
// that must be gone is marked tools:node="remove" - the merge then drops it
// even when a library declares it. This pins the list: the ones the TV never
// uses go, the two playback needs stay.
// The plugin is CommonJS and requires expo/config-plugins itself, which a
// vitest mock does not reach: put the stand-in in the require cache instead.
// withAndroidManifest here runs the callback at once on the config it is given.
const require_ = createRequire(import.meta.url);
const configPluginsPath = require_.resolve("expo/config-plugins");
require_.cache[configPluginsPath] = {
  id: configPluginsPath,
  filename: configPluginsPath,
  loaded: true,
  exports: { withAndroidManifest: (config: unknown, fn: (c: unknown) => unknown) => fn(config) },
} as unknown as NodeJS.Module;
type Manifest = { $?: Record<string, string>; "uses-permission"?: { $: Record<string, string> }[]; "uses-feature"?: { $: Record<string, string> }[] };

function run(initial: string[]): Manifest {
  const plugin = require_("../../plugins/withTvHardwareFeatures.js") as (c: unknown) => { modResults: { manifest: Manifest } };
  const manifest: Manifest = { "uses-permission": initial.map((n) => ({ $: { "android:name": n } })) };
  return plugin({ modResults: { manifest } }).modResults.manifest;
}

const removed = (m: Manifest) =>
  (m["uses-permission"] ?? []).filter((p) => p.$["tools:node"] === "remove").map((p) => p.$["android:name"]).sort();

describe("withTvHardwareFeatures", () => {
  it("removes what the television never uses, declared or not", () => {
    const m = run(["android.permission.SYSTEM_ALERT_WINDOW", "android.permission.INTERNET"]);
    expect(removed(m)).toEqual(
      [
        "android.permission.RECORD_AUDIO",
        "android.permission.READ_EXTERNAL_STORAGE",
        "android.permission.SYSTEM_ALERT_WINDOW",
        "android.permission.USE_BIOMETRIC",
        "android.permission.USE_FINGERPRINT",
        "android.permission.WRITE_EXTERNAL_STORAGE",
      ].sort(),
    );
  });

  it("keeps what playback needs", () => {
    const m = run(["android.permission.FOREGROUND_SERVICE", "android.permission.WAKE_LOCK", "android.permission.INTERNET"]);
    const kept = (m["uses-permission"] ?? []).filter((p) => !p.$["tools:node"]).map((p) => p.$["android:name"]);
    expect(kept).toEqual(["android.permission.FOREGROUND_SERVICE", "android.permission.WAKE_LOCK", "android.permission.INTERNET"]);
    expect(m.$?.["xmlns:tools"]).toBe("http://schemas.android.com/tools");
  });

  it("keeps leanback required and the hardware a television lacks optional", () => {
    const m = run([]);
    const optional = (m["uses-feature"] ?? []).filter((f) => f.$["android:required"] === "false").map((f) => f.$["android:name"]);
    expect(optional).toContain("android.hardware.touchscreen");
    expect(optional).not.toContain("android.software.leanback");
  });
});
