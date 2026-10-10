import { describe, it, expect, vi, beforeEach } from "vitest";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

const openURL = vi.fn();
vi.mock("react-native", () => ({ Linking: { openURL: (...a: unknown[]) => openURL(...a) } }));

import { newerMobileRelease, openStorePage, INSTALLED_VERSION_CODE } from "./appUpdate";

beforeEach(() => openURL.mockReset());

describe("newerMobileRelease", () => {
  const health = (clients: unknown) => ({ status: "ok", clients });

  // 10-10: a phone a month on beta.74 while beta.79 was out.
  it("offers a phone build newer than the installed one, never the TV's", () => {
    const h = health({ tv: { version: "0.1.21", version_code: 86 }, mobile: { version: "0.0.1-beta.79", version_code: 85 } });
    expect(newerMobileRelease(h, 76)).toEqual({ version: "0.0.1-beta.79", versionCode: 85 });
    expect(newerMobileRelease(h, 85)).toBeNull();
    expect(newerMobileRelease(health({ tv: { version: "0.1.21", version_code: 86 } }), 76)).toBeNull();
  });

  it("compares against this build's own versionCode by default", () => {
    expect(newerMobileRelease(health({ mobile: { version: "next", version_code: INSTALLED_VERSION_CODE + 1 } }))).not.toBeNull();
    expect(newerMobileRelease(health({ mobile: { version: "same", version_code: INSTALLED_VERSION_CODE } }))).toBeNull();
  });
});

// The server stamps the app.json versionCode; Gradle builds the APK with its
// own. A release bump that moves one without the other would make the banner
// nag forever (app.json ahead) or never show (behind).
it("keeps app.json's versionCode equal to the one Gradle builds", () => {
  const gradle = readFileSync(resolve(__dirname, "../../android/app/build.gradle"), "utf8");
  const code = /^\s*versionCode\s+(\d+)/m.exec(gradle)?.[1];
  expect(code, "versionCode in android/app/build.gradle").toBeDefined();
  expect(Number(code)).toBe(INSTALLED_VERSION_CODE);
  const appJson = JSON.parse(readFileSync(resolve(__dirname, "../../app.json"), "utf8")) as { expo: { version: string } };
  expect(/versionName\s+"([^"]+)"/.exec(gradle)?.[1]).toBe(appJson.expo.version);
});

describe("openStorePage", () => {
  it("opens the Play Store app", async () => {
    openURL.mockResolvedValue(undefined);
    await openStorePage();
    expect(openURL).toHaveBeenCalledWith("market://details?id=com.evey.app");
  });

  it("falls back to the web page when no store app answers", async () => {
    openURL.mockRejectedValueOnce(new Error("no activity")).mockResolvedValueOnce(undefined);
    await openStorePage();
    expect(openURL).toHaveBeenLastCalledWith("https://play.google.com/store/apps/details?id=com.evey.app");
  });
});
