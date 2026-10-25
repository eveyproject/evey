import { beforeEach, describe, expect, it, vi } from "vitest";
import type { DeviceProfile } from "@evey/media-caps";

const native = vi.hoisted(() => ({
  profile: null as DeviceProfile | null,
  stored: [] as string[],
}));

vi.mock("@evey/media-caps", () => ({
  getDeviceProfile: () => native.profile,
  setLiteMode: (mode: string) => {
    native.stored.push(mode);
    return true;
  },
}));

import { chooseLiteMode, decideLite, isLiteMode, LITE_TOTAL_MB, liteModeChoice, resetLiteModeForTests } from "./liteMode";

const profile = (over: Partial<DeviceProfile>): DeviceProfile => ({
  lowRam: false,
  totalMb: 3800,
  memoryClassMb: 256,
  liteMode: "auto",
  ...over,
});

describe("liteMode", () => {
  beforeEach(() => {
    native.profile = null;
    native.stored = [];
    resetLiteModeForTests();
  });

  it("auto: a 2 GB box stays full, less memory or the platform's low-RAM flag turns it on", () => {
    // A 2 GB box reports about 1.9 GB of total memory, a 1.5 GB one about 1.4.
    expect(decideLite(profile({ totalMb: 1900 }))).toBe(false);
    expect(decideLite(profile({ totalMb: 1400 }))).toBe(true);
    expect(decideLite(profile({ totalMb: LITE_TOTAL_MB }))).toBe(false);
    expect(decideLite(profile({ totalMb: 3800, lowRam: true }))).toBe(true);
  });

  it("the user's choice wins over the memory, both ways", () => {
    expect(decideLite(profile({ totalMb: 1400, liteMode: "off" }))).toBe(false);
    expect(decideLite(profile({ totalMb: 3800, liteMode: "on" }))).toBe(true);
  });

  it("no native module (web, tvOS, an older build): full mode", () => {
    expect(decideLite(null)).toBe(false);
    expect(isLiteMode()).toBe(false);
    expect(liteModeChoice()).toBe("auto");
  });

  it("choosing persists natively and applies at once", () => {
    native.profile = profile({ totalMb: 1400 });
    expect(isLiteMode()).toBe(true);
    chooseLiteMode("off");
    expect(native.stored).toEqual(["off"]);
    expect(isLiteMode()).toBe(false);
    expect(liteModeChoice()).toBe("off");
  });
});
