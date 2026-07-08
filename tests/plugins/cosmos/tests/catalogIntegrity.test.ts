import { describe, it, expect } from "vitest";
import { BRIGHT_STARS } from "../src/domain/data/brightStars";
import { CONSTELLATIONS } from "../src/domain/data/constellations";

describe("BRIGHT_STARS", () => {
  it("has unique ids", () => {
    const ids = BRIGHT_STARS.map((s) => s.id);
    expect(new Set(ids).size).toBe(ids.length);
  });

  it("has plausible coordinate ranges for every star", () => {
    for (const s of BRIGHT_STARS) {
      expect(s.raH).toBeGreaterThanOrEqual(0);
      expect(s.raH).toBeLessThan(24);
      expect(s.decDeg).toBeGreaterThanOrEqual(-90);
      expect(s.decDeg).toBeLessThanOrEqual(90);
      expect(s.mag).toBeGreaterThan(-2);
      expect(s.mag).toBeLessThan(7);
    }
  });

  it("covers at least 15 distinct constellations (spec minimum)", () => {
    const cons = new Set(BRIGHT_STARS.map((s) => s.con));
    expect(cons.size).toBeGreaterThanOrEqual(15);
  });
});

describe("CONSTELLATIONS", () => {
  const starIds = new Set(BRIGHT_STARS.map((s) => s.id));

  it("has at least 15 constellations (spec requirement)", () => {
    expect(CONSTELLATIONS.length).toBeGreaterThanOrEqual(15);
  });

  it("every line segment references stars that exist in BRIGHT_STARS", () => {
    for (const con of CONSTELLATIONS) {
      for (const [a, b] of con.segments) {
        expect(starIds.has(a), `${con.id}: unknown star id "${a}"`).toBe(true);
        expect(starIds.has(b), `${con.id}: unknown star id "${b}"`).toBe(true);
      }
    }
  });

  it("every constellation has at least one segment", () => {
    for (const con of CONSTELLATIONS) {
      expect(con.segments.length).toBeGreaterThan(0);
    }
  });
});
