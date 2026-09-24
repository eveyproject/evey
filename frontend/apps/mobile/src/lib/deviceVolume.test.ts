import { describe, it, expect, vi, beforeEach } from "vitest";

const getDeviceVolume = vi.fn(async () => 0.4);
const getDeviceVolumeSteps = vi.fn(async () => 15);
// The device quantizes: 15 steps means a drag lands on multiples of 1/15.
const setDeviceVolume = vi.fn(async (f: number) => Math.round(f * 15) / 15);

vi.mock("react-native", () => ({
  Platform: { OS: "android" },
  NativeModules: {
    get AudioBoost() {
      return { getDeviceVolume, getDeviceVolumeSteps, setDeviceVolume, setBoost: vi.fn(), release: vi.fn() };
    },
  },
}));

const mod = await import("./deviceVolume");

beforeEach(() => {
  getDeviceVolume.mockClear();
  getDeviceVolumeSteps.mockClear();
  getDeviceVolumeSteps.mockImplementation(async () => 15);
  setDeviceVolume.mockClear();
  setDeviceVolume.mockImplementation(async (f: number) => Math.round(f * 15) / 15);
  mod._resetDeviceVolumeForTest();
});

describe("hasDeviceVolumeControl", () => {
  it("is true when the native bridge exposes the methods", () => {
    expect(mod.hasDeviceVolumeControl()).toBe(true);
  });
});

describe("setDeviceVolumeFraction", () => {
  it("collapses a 60-event drag into few native calls", async () => {
    const drag = Array.from({ length: 60 }, (_, i) => 1 - i / 60);
    await Promise.all(drag.map((v) => mod.setDeviceVolumeFraction(v)));
    // 15 device steps is the ceiling worth sending; anything more is bridge churn.
    expect(setDeviceVolume.mock.calls.length).toBeLessThanOrEqual(20);
  });

  it("always ends on the value the user last asked for", async () => {
    await Promise.all([1, 0.8, 0.6, 0.4, 0.05].map((v) => mod.setDeviceVolumeFraction(v)));
    expect(setDeviceVolume.mock.calls.at(-1)?.[0]).toBeCloseTo(0.05, 2);
  });

  it("never overlaps native calls", async () => {
    let inFlight = 0;
    let overlapped = false;
    setDeviceVolume.mockImplementation(async (f: number) => {
      inFlight++;
      if (inFlight > 1) overlapped = true;
      await new Promise((r) => setTimeout(r, 1));
      inFlight--;
      return f;
    });
    await Promise.all([0.2, 0.4, 0.6, 0.8].map((v) => mod.setDeviceVolumeFraction(v)));
    expect(overlapped).toBe(false);
  });

  it("reports the level the device really reached, not the one requested", async () => {
    // 0.5 * 15 = 7.5 -> index 8 -> 0.5333...
    const applied = await mod.setDeviceVolumeFraction(0.5);
    expect(applied).toBeCloseTo(8 / 15, 3);
  });

  it("ends on the value asked last even when it equals the one applied before", async () => {
    await mod.setDeviceVolumeFraction(0.6);
    setDeviceVolume.mockImplementation(async (f: number) => { await new Promise((r) => setTimeout(r, 1)); return f; });
    await Promise.all([0.667, 0.733, 0.6].map((v) => mod.setDeviceVolumeFraction(v)));
    expect(setDeviceVolume.mock.calls.at(-1)?.[0]).toBeCloseTo(0.6, 3);
  });

  // The hardware keys move the device behind the bar's back: once the level
  // has been read, asking for the old value again is a real change.
  it("is not skipped after the hardware keys moved the device", async () => {
    await mod.setDeviceVolumeFraction(0.6);
    mod.noteObservedDeviceVolume(7 / 15);
    setDeviceVolume.mockClear();
    await mod.setDeviceVolumeFraction(0.6);
    expect(setDeviceVolume).toHaveBeenCalledWith(0.6);
  });

  it("survives a refusal (Do-Not-Disturb) without wedging", async () => {
    setDeviceVolume.mockRejectedValueOnce(new Error("forbidden"));
    await expect(mod.setDeviceVolumeFraction(0.3)).resolves.toBeNull();
    setDeviceVolume.mockImplementation(async (f: number) => f);
    await expect(mod.setDeviceVolumeFraction(0.7)).resolves.toBeCloseTo(0.7, 3);
  });
});

describe("readDeviceVolumeFraction", () => {
  it("returns the current device level", async () => {
    await expect(mod.readDeviceVolumeFraction()).resolves.toBeCloseTo(0.4, 3);
  });

  it("returns null when the device refuses, so the caller keeps its own value", async () => {
    getDeviceVolume.mockRejectedValueOnce(new Error("nope"));
    await expect(mod.readDeviceVolumeFraction()).resolves.toBeNull();
  });
});

describe("readDeviceVolumeSteps", () => {
  it("returns the hardware step count and caches it (one native call)", async () => {
    expect(await mod.readDeviceVolumeSteps()).toBe(15);
    expect(await mod.readDeviceVolumeSteps()).toBe(15);
    expect(getDeviceVolumeSteps).toHaveBeenCalledTimes(1);
  });

  it("answers null - and remembers it - when the bridge refuses", async () => {
    getDeviceVolumeSteps.mockImplementation(async () => { throw new Error("no AudioManager"); });
    expect(await mod.readDeviceVolumeSteps()).toBe(null);
    expect(await mod.readDeviceVolumeSteps()).toBe(null);
    expect(getDeviceVolumeSteps).toHaveBeenCalledTimes(1);
  });

  it("rejects a nonsensical step count instead of feeding it to the policy", async () => {
    getDeviceVolumeSteps.mockImplementation(async () => 0);
    expect(await mod.readDeviceVolumeSteps()).toBe(null);
  });
});

