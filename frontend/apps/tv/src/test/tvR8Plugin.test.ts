import { createRequire } from "node:module";
import { mkdtempSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, it, expect } from "vitest";

// R8 on the TV lives in a config plugin because apps/tv/android is a prebuild
// artifact that git ignores. Pin what the plugin writes: the two Gradle
// switches, and the keep rules (once, even if prebuild runs twice).
const require_ = createRequire(import.meta.url);
type Prop = { type: string; key: string; value: string };
const configPluginsPath = require_.resolve("expo/config-plugins");
let darkMod: ((cfg: unknown) => Promise<unknown>) | undefined;
let propsMod: ((cfg: { modResults: Prop[] }) => unknown) | undefined;
require_.cache[configPluginsPath] = {
  id: configPluginsPath,
  filename: configPluginsPath,
  loaded: true,
  exports: {
    withGradleProperties: (config: unknown, fn: typeof propsMod) => ((propsMod = fn), config),
    withDangerousMod: (config: unknown, [, fn]: [string, typeof darkMod]) => ((darkMod = fn), config),
  },
} as unknown as NodeJS.Module;
const plugin = require_("../../plugins/withR8Minify.js") as ((c: unknown) => unknown) & { MARK: string };

describe("withR8Minify", () => {
  it("turns minification and resource shrinking on, replacing a previous value", () => {
    plugin({});
    const props: Prop[] = [{ type: "property", key: "android.enableMinifyInReleaseBuilds", value: "false" }];
    propsMod!({ modResults: props });
    const value = (k: string) => props.find((p) => p.key === k)?.value;
    expect(value("android.enableMinifyInReleaseBuilds")).toBe("true");
    expect(value("android.enableShrinkResourcesInReleaseBuilds")).toBe("true");
    expect(props.filter((p) => p.key === "android.enableMinifyInReleaseBuilds")).toHaveLength(1);
  });

  it("appends the keep rules to proguard-rules.pro once", async () => {
    plugin({});
    const root = mkdtempSync(join(tmpdir(), "r8-"));
    mkdirSync(join(root, "app"));
    const file = join(root, "app", "proguard-rules.pro");
    writeFileSync(file, "# generated\n");
    for (let i = 0; i < 2; i++) await darkMod!({ modRequest: { platformProjectRoot: root } });
    const text = readFileSync(file, "utf8");
    expect(text.startsWith("# generated")).toBe(true);
    expect(text.split(plugin.MARK).length - 1).toBe(1);
    expect(text).toContain("-keep @interface expo.modules.kotlin.records.Field");
    expect(text).toContain("-keep @interface expo.modules.kotlin.records.Required");
  });
});
