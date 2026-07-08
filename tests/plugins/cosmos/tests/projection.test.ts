import { describe, it, expect } from "vitest";
import { stereographicProject } from "../src/domain/services/projection";

describe("stereographicProject", () => {
  it("maps the zenith to the center of the disc", () => {
    const p = stereographicProject(90, 123);
    expect(p.x).toBeCloseTo(0, 6);
    expect(p.y).toBeCloseTo(0, 6);
    expect(p.visible).toBe(true);
  });

  it("maps the horizon exactly to the unit circle", () => {
    for (const az of [0, 45, 90, 180, 270, 359]) {
      const p = stereographicProject(0, az);
      const r = Math.hypot(p.x, p.y);
      expect(r).toBeCloseTo(1, 6);
    }
  });

  it("maps north (az=0) to +y and east (az=90) to +x", () => {
    const north = stereographicProject(0, 0);
    expect(north.x).toBeCloseTo(0, 6);
    expect(north.y).toBeCloseTo(1, 6);

    const east = stereographicProject(0, 90);
    expect(east.x).toBeCloseTo(1, 6);
    expect(east.y).toBeCloseTo(0, 6);
  });

  it("places below-horizon points outside the unit disc and flags them not visible", () => {
    const p = stereographicProject(-10, 0);
    expect(Math.hypot(p.x, p.y)).toBeGreaterThan(1);
    expect(p.visible).toBe(false);
  });

  it("radius grows monotonically as altitude drops toward the horizon", () => {
    const radii = [90, 60, 30, 10, 1].map((alt) => {
      const { x, y } = stereographicProject(alt, 0);
      return Math.hypot(x, y);
    });
    for (let i = 1; i < radii.length; i++) {
      expect(radii[i]).toBeGreaterThan(radii[i - 1]);
    }
  });
});
