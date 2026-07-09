import { describe, it, expect } from "vitest";
import { PRESETS, PRESET_IDS, DEFAULT_PRESET, isPresetId, presetOrDefault } from "../src/domain/services/presets";

describe("presets", () => {
  it("has exactly the four named presets", () => {
    expect(PRESET_IDS.sort()).toEqual(["aurore", "braise", "neon", "ocean"]);
  });

  it("every preset has at least 3 anchors and finite 0..1 channels", () => {
    for (const id of PRESET_IDS) {
      const preset = PRESETS[id];
      expect(preset.anchors.length).toBeGreaterThanOrEqual(3);
      for (const c of [...preset.anchors, preset.vignette]) {
        for (const v of c) {
          expect(Number.isFinite(v)).toBe(true);
          expect(v).toBeGreaterThanOrEqual(0);
          expect(v).toBeLessThanOrEqual(1);
        }
      }
    }
  });

  it("presets are visually distinct (no two share the same first anchor)", () => {
    const firstAnchors = PRESET_IDS.map((id) => PRESETS[id].anchors[0].join(","));
    expect(new Set(firstAnchors).size).toBe(PRESET_IDS.length);
  });

  it("isPresetId accepts only the four known ids", () => {
    for (const id of PRESET_IDS) expect(isPresetId(id)).toBe(true);
    expect(isPresetId("sunset")).toBe(false);
    expect(isPresetId(null)).toBe(false);
    expect(isPresetId(42)).toBe(false);
  });

  it("presetOrDefault falls back to the default for unknown input", () => {
    expect(presetOrDefault("neon")).toBe("neon");
    expect(presetOrDefault("bogus")).toBe(DEFAULT_PRESET);
    expect(presetOrDefault(null)).toBe(DEFAULT_PRESET);
    expect(presetOrDefault(undefined)).toBe(DEFAULT_PRESET);
  });
});
