import { describe, it, expect } from "vitest";
import { resolveCity, CITY_GAZETTEER } from "../src/domain/data/cityGazetteer";

describe("resolveCity", () => {
  it("resolves an exact, case-insensitive match", () => {
    const c = resolveCity("tokyo");
    expect(c.name).toBe("Tokyo");
    expect(c.lat).toBeCloseTo(35.6762, 2);
  });

  it("resolves accented input against unaccented gazetteer entries", () => {
    const c = resolveCity("Zürich");
    expect(c.name).toBe("Zurich");
  });

  it("falls back to Paris for an unknown city", () => {
    const c = resolveCity("Nowhereville");
    expect(c.name).toBe("Paris");
  });

  it("falls back to Paris for empty/undefined input", () => {
    expect(resolveCity("").name).toBe("Paris");
    expect(resolveCity(undefined).name).toBe("Paris");
    expect(resolveCity(null).name).toBe("Paris");
  });

  it("every gazetteer entry has valid coordinates", () => {
    for (const c of CITY_GAZETTEER) {
      expect(c.lat).toBeGreaterThanOrEqual(-90);
      expect(c.lat).toBeLessThanOrEqual(90);
      expect(c.lon).toBeGreaterThanOrEqual(-180);
      expect(c.lon).toBeLessThanOrEqual(180);
    }
  });
});
