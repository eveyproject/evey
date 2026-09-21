import { describe, expect, it, vi, beforeEach } from "vitest";

const getItemAsync = vi.fn();
const setItemAsync = vi.fn();
vi.mock("expo-secure-store", () => ({
  getItemAsync: (...args: unknown[]) => getItemAsync(...args),
  setItemAsync: (...args: unknown[]) => setItemAsync(...args),
}));

import {
  DEFAULT_SUBTITLE_STYLE,
  getSubtitleStylePref,
  saveSubtitleStylePref,
  subtitleBgRgba,
  subtitleCueFontSizePx,
} from "./subtitleStyle";

beforeEach(() => {
  getItemAsync.mockReset();
  setItemAsync.mockReset();
});

describe("subtitleCueFontSizePx", () => {
  it("scales with container height and the fontSize percentage", () => {
    expect(subtitleCueFontSizePx(1000, 100)).toBeCloseTo(50);
    expect(subtitleCueFontSizePx(1000, 200)).toBeCloseTo(100);
  });

  it("floors at the minimum readable size", () => {
    expect(subtitleCueFontSizePx(100, 50)).toBe(13);
  });

  it("returns 0 for a not-yet-measured container", () => {
    expect(subtitleCueFontSizePx(0, 100)).toBe(0);
  });
});

describe("subtitleBgRgba", () => {
  it("converts a hex color + opacity percent to an rgba string", () => {
    expect(subtitleBgRgba("#000000", 75)).toBe("rgba(0,0,0,0.75)");
    expect(subtitleBgRgba("#1e3a5f", 0)).toBe("rgba(30,58,95,0)");
  });

  it("falls back to black for an invalid color", () => {
    expect(subtitleBgRgba("not-a-color", 50)).toBe("rgba(0,0,0,0.5)");
  });
});

describe("DEFAULT_SUBTITLE_STYLE", () => {
  it("matches the web defaults minus fontFamily", () => {
    expect(DEFAULT_SUBTITLE_STYLE).toEqual({
      fontSize: 100,
      color: "#ffffff",
      bgColor: "#000000",
      bgOpacity: 75,
      position: "bottom",
      shadow: false,
    });
  });
});

describe("getSubtitleStylePref", () => {
  it("returns null when nothing is stored", async () => {
    getItemAsync.mockResolvedValue(null);
    expect(await getSubtitleStylePref()).toBeNull();
  });

  it("merges a stored partial style over the defaults", async () => {
    getItemAsync.mockResolvedValue(JSON.stringify({ fontSize: 150 }));
    expect(await getSubtitleStylePref()).toEqual({ ...DEFAULT_SUBTITLE_STYLE, fontSize: 150 });
  });

  it("returns null on corrupted JSON instead of throwing", async () => {
    getItemAsync.mockResolvedValue("{not json");
    expect(await getSubtitleStylePref()).toBeNull();
  });

  it("returns null when the keystore read rejects", async () => {
    getItemAsync.mockRejectedValue(new Error("no keystore"));
    expect(await getSubtitleStylePref()).toBeNull();
  });
});

describe("saveSubtitleStylePref", () => {
  it("writes the style as JSON under the evey-subtitle-style key", async () => {
    const style = { ...DEFAULT_SUBTITLE_STYLE, fontSize: 150 };
    await saveSubtitleStylePref(style);
    expect(setItemAsync).toHaveBeenCalledWith("evey-subtitle-style", JSON.stringify(style));
  });

  it("does not throw when the keystore write rejects", async () => {
    setItemAsync.mockRejectedValue(new Error("no keystore"));
    await expect(saveSubtitleStylePref(DEFAULT_SUBTITLE_STYLE)).resolves.toBeUndefined();
  });
});
