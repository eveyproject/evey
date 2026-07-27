import { describe, it, expect } from "vitest";
import { regionOptions } from "./regionOptions";

describe("regionOptions", () => {
  it("returns two-letter codes with localized labels, sorted alphabetically", () => {
    const opts = regionOptions("en");
    expect(opts.length).toBeGreaterThan(50);
    expect(opts.every((o) => /^[A-Z]{2}$/.test(o.code))).toBe(true);
    const sorted = [...opts].sort((a, b) => a.label.localeCompare(b.label, "en"));
    expect(opts).toEqual(sorted);
  });

  it("includes common markets missing from the old truncated fallback list", () => {
    // Regression: EditProfileDialog used to have its own shorter FALLBACK_REGIONS
    // missing several ISO-3166 codes (e.g. Iceland, Luxembourg) that the region
    // settings screen already supported.
    const codes = regionOptions("en").map((o) => o.code);
    expect(codes).toContain("IS");
    expect(codes).toContain("LU");
  });
});
