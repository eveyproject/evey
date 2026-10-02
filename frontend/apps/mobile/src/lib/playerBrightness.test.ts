import { beforeEach, describe, expect, it, vi } from "vitest";

const b = vi.hoisted(() => ({
  BrightnessMode: { UNKNOWN: 0, AUTOMATIC: 1, MANUAL: 2 },
  setBrightnessAsync: vi.fn(async (_v: number) => {}),
  getBrightnessAsync: vi.fn(async () => 0),
  isUsingSystemBrightnessAsync: vi.fn(async () => true),
  getSystemBrightnessModeAsync: vi.fn(async () => 1),
  restoreSystemBrightnessAsync: vi.fn(async () => {}),
}));
vi.mock("expo-brightness", () => b);

import {
  _resetPlayerBrightnessForTest,
  openingPlayerBrightness,
  releasePlayerBrightness,
  setPlayerBrightness,
} from "./playerBrightness";

describe("playerBrightness", () => {
  beforeEach(() => {
    _resetPlayerBrightnessForTest();
    vi.clearAllMocks();
    b.isUsingSystemBrightnessAsync.mockResolvedValue(true);
    b.getSystemBrightnessModeAsync.mockResolvedValue(b.BrightnessMode.AUTOMATIC);
    b.getBrightnessAsync.mockResolvedValue(0);
  });

  it("adaptive brightness opens at the middle, not the 0% expo-brightness reads", async () => {
    expect(await openingPlayerBrightness()).toBe(0.5);
    expect(b.setBrightnessAsync).not.toHaveBeenCalled();
  });

  it("manual brightness opens at the system level", async () => {
    b.getSystemBrightnessModeAsync.mockResolvedValue(b.BrightnessMode.MANUAL);
    b.getBrightnessAsync.mockResolvedValue(0.7);
    expect(await openingPlayerBrightness()).toBe(0.7);
  });

  it("a level the activity already overrides is read as is", async () => {
    b.isUsingSystemBrightnessAsync.mockResolvedValue(false);
    b.getBrightnessAsync.mockResolvedValue(0.3);
    expect(await openingPlayerBrightness()).toBe(0.3);
  });

  it("coming back to a title reapplies the level picked before leaving", async () => {
    await setPlayerBrightness(0.8);
    await releasePlayerBrightness();
    b.setBrightnessAsync.mockClear();
    expect(await openingPlayerBrightness()).toBe(0.8);
    expect(b.setBrightnessAsync).toHaveBeenCalledWith(0.8);
  });

  it("leaving the player hands the screen back to the system", async () => {
    await releasePlayerBrightness();
    expect(b.restoreSystemBrightnessAsync).toHaveBeenCalledTimes(1);
  });
});
