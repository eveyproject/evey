import { describe, it, expect, vi, beforeEach } from "vitest";

const openURL = vi.fn();
vi.mock("react-native", () => ({ Linking: { openURL: (...a: unknown[]) => openURL(...a) } }));

import { newerTVRelease, openStorePage, INSTALLED_VERSION_CODE } from "./appUpdate";

beforeEach(() => openURL.mockReset());

describe("newerTVRelease", () => {
  const health = (tv: unknown) => ({ status: "ok", clients: { tv } });

  // 27-09: a TV sat a week on 0.1.4 while 0.1.11 was out.
  it("offers a build newer than the installed one", () => {
    expect(newerTVRelease(health({ version: "0.1.11", version_code: 56 }), 23)).toEqual({ version: "0.1.11", versionCode: 56 });
  });

  it("offers nothing for the installed build or an older one", () => {
    expect(newerTVRelease(health({ version: "0.1.11", version_code: 56 }), 56)).toBeNull();
    expect(newerTVRelease(health({ version: "0.1.10", version_code: 48 }), 56)).toBeNull();
  });

  it("offers nothing when an older or unstamped server says nothing", () => {
    expect(newerTVRelease({ status: "ok" }, 23)).toBeNull();
    expect(newerTVRelease(null, 23)).toBeNull();
    expect(newerTVRelease(health({ version: "x", version_code: "56" }), 23)).toBeNull();
    expect(newerTVRelease(health({ version: "x", version_code: 56.5 }), 23)).toBeNull();
  });

  it("compares against this build's own versionCode by default", () => {
    expect(newerTVRelease(health({ version: "next", version_code: INSTALLED_VERSION_CODE + 1 }))).not.toBeNull();
    expect(newerTVRelease(health({ version: "same", version_code: INSTALLED_VERSION_CODE }))).toBeNull();
  });
});

describe("openStorePage", () => {
  it("opens the Play Store app on the TV", async () => {
    openURL.mockResolvedValue(undefined);
    await openStorePage();
    expect(openURL).toHaveBeenCalledWith("market://details?id=com.evey.tv");
  });

  it("falls back to the web page when no store app answers", async () => {
    openURL.mockRejectedValueOnce(new Error("no activity")).mockResolvedValueOnce(undefined);
    await openStorePage();
    expect(openURL).toHaveBeenLastCalledWith("https://play.google.com/store/apps/details?id=com.evey.tv");
  });
});
