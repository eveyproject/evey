import { describe, it, expect } from "vitest";
import { readdirSync, readFileSync, existsSync } from "node:fs";
import { join, resolve } from "node:path";

// The ac3go renderer only reaches the player through the expo-video PATCH
// (buildAudioRenderers). An app that takes expo-video's prebuilt AAR instead
// still declares ac3/eac3 to the server (playbackDecoders) but plays the video
// SILENT: measured on the TV emulator, 2026-09-26, before this guard.
const APPS = resolve(__dirname, "../../../apps");

describe("apps embedding @evey/offline-audio", () => {
  const apps = readdirSync(APPS)
    .map((name) => join(APPS, name, "package.json"))
    .filter(existsSync)
    .map((path) => ({ path, pkg: JSON.parse(readFileSync(path, "utf8")) }))
    .filter(({ pkg }) => pkg.dependencies?.["@evey/offline-audio"]);

  it("covers the phone and the TV", () => {
    expect(apps.map(({ pkg }) => pkg.name).sort()).toEqual(["mobile", "tv"]);
  });

  it.each(apps.map(({ path, pkg }) => [path, pkg]))("%s builds expo-video from source", (_path, pkg) => {
    expect(pkg.expo?.autolinking?.android?.buildFromSource).toContain("expo-video");
  });
});
