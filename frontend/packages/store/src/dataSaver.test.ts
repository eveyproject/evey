import { beforeEach, describe, expect, it } from "vitest";
import { currentDataSaverLimits, dataSaverLabelValues, dataSaverLimits, tighterCeiling, useDataSaverStore } from "./dataSaver";

describe("dataSaverLimits", () => {
  it("puts no ceiling when off", () => {
    expect(dataSaverLimits("off")).toBeNull();
  });

  it("travel is tighter than saver on every axis", () => {
    const saver = dataSaverLimits("saver")!;
    const travel = dataSaverLimits("travel")!;
    expect(travel.maxBitrateKbps).toBeLessThan(saver.maxBitrateKbps);
    expect(travel.maxHeight).toBeLessThan(saver.maxHeight);
    expect(travel.maxAudioChannels).toBe(2);
    expect(saver.maxAudioChannels).toBe(2);
  });
});

describe("tighterCeiling", () => {
  it("keeps the lower of two set ceilings, both ways", () => {
    expect(tighterCeiling(720, 1080)).toBe(720);
    expect(tighterCeiling(1080, 720)).toBe(720);
  });

  it("treats 0 and undefined as no ceiling", () => {
    expect(tighterCeiling(undefined, 480)).toBe(480);
    expect(tighterCeiling(480, 0)).toBe(480);
    expect(tighterCeiling(0, undefined)).toBeUndefined();
  });
});

describe("useDataSaverStore", () => {
  beforeEach(() => useDataSaverStore.setState({ level: "off" }));

  it("defaults to off and follows the chosen level", () => {
    expect(currentDataSaverLimits()).toBeNull();
    useDataSaverStore.getState().setLevel("travel");
    expect(currentDataSaverLimits()).toEqual(dataSaverLimits("travel"));
  });

  it("reads an unknown stored level as off", async () => {
    localStorage.setItem("evey-data-saver", JSON.stringify({ state: { level: "ultra" }, version: 0 }));
    await useDataSaverStore.persist.rehydrate();
    expect(useDataSaverStore.getState().level).toBe("off");
  });
});

describe("dataSaverLabelValues", () => {
  it("names the height and the hourly cost, video plus the stereo track", () => {
    expect(dataSaverLabelValues("saver", "en")).toEqual({ height: 720, gb: "1.4" });
    expect(dataSaverLabelValues("travel", "fr")).toEqual({ height: 480, gb: "0,5" });
    expect(dataSaverLabelValues("off", "en")).toEqual({});
  });
});
