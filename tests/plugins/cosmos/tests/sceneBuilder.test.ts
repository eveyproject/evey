import { describe, it, expect } from "vitest";
import { buildSkyScene } from "../src/domain/services/sceneBuilder";

const PARIS = { latDeg: 48.8566, lonDeg: 2.3522 };

describe("buildSkyScene", () => {
  it("produces a fully-populated, internally consistent scene", () => {
    const scene = buildSkyScene(Date.UTC(2024, 5, 21, 22, 0, 0), PARIS);
    expect(scene.stars.length).toBeGreaterThan(50);
    expect(scene.lines.length).toBeGreaterThan(15);
    expect(scene.planets.length).toBe(4);
    expect(scene.starVisibility).toBeGreaterThanOrEqual(0);
    expect(scene.starVisibility).toBeLessThanOrEqual(1);
    expect(scene.moon.phaseFraction).toBeGreaterThanOrEqual(0);
    expect(scene.moon.phaseFraction).toBeLessThanOrEqual(1);
  });

  it("shows more visible stars at night than at noon, for the same date/place", () => {
    const noon = buildSkyScene(Date.UTC(2024, 5, 21, 12, 0, 0), PARIS);
    const midnight = buildSkyScene(Date.UTC(2024, 5, 21, 23, 0, 0), PARIS);
    expect(noon.mood).toBe("day");
    expect(midnight.starVisibility).toBeGreaterThan(noon.starVisibility);
  });

  it("keeps every star/line coordinate finite (no NaN from degenerate trig)", () => {
    const scene = buildSkyScene(Date.UTC(2024, 0, 1, 0, 0, 0), PARIS);
    for (const s of scene.stars) {
      expect(Number.isFinite(s.x)).toBe(true);
      expect(Number.isFinite(s.y)).toBe(true);
    }
    for (const l of scene.lines) {
      expect(Number.isFinite(l.ax)).toBe(true);
      expect(Number.isFinite(l.ay)).toBe(true);
    }
  });

  it("computes one label centroid per constellation, all finite", () => {
    const scene = buildSkyScene(Date.UTC(2024, 5, 21, 22, 0, 0), PARIS);
    expect(scene.constellationLabels.length).toBe(18);
    for (const l of scene.constellationLabels) {
      expect(Number.isFinite(l.x)).toBe(true);
      expect(Number.isFinite(l.y)).toBe(true);
      expect(l.labelKey).toBeTruthy();
    }
  });

  it("keeps working for a southern-hemisphere observer", () => {
    const sydney = buildSkyScene(Date.UTC(2024, 5, 21, 12, 0, 0), { latDeg: -33.8688, lonDeg: 151.2093 });
    expect(sydney.stars.length).toBeGreaterThan(50);
    expect(Number.isFinite(sydney.moon.x)).toBe(true);
  });
});
