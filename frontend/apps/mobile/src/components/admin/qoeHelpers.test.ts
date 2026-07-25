import { describe, it, expect } from "vitest";
import { formatPct, formatInt, formatRelative, qualityStatus } from "./qoeHelpers";

describe("formatPct", () => {
  it("renders 0% for non-finite or non-positive ratios", () => {
    expect(formatPct(0)).toBe("0%");
    expect(formatPct(-1)).toBe("0%");
    expect(formatPct(NaN)).toBe("0%");
  });

  it("renders two decimal places", () => {
    expect(formatPct(0.0512)).toBe("5.12%");
    expect(formatPct(1)).toBe("100.00%");
  });
});

describe("formatInt", () => {
  it("renders a dash for undefined/null/non-finite", () => {
    expect(formatInt(undefined)).toBe("-");
    expect(formatInt(NaN)).toBe("-");
  });

  it("renders locale-formatted integers", () => {
    expect(formatInt(1234)).toBe((1234).toLocaleString());
    expect(formatInt(0)).toBe("0");
  });
});

describe("formatRelative", () => {
  it("renders a dash for empty input", () => {
    expect(formatRelative(undefined, "now")).toBe("-");
    expect(formatRelative("", "now")).toBe("-");
  });

  it("renders a dash for an unparsable timestamp", () => {
    expect(formatRelative("not-a-date", "now")).toBe("-");
  });

  it("buckets into just-now/minutes/hours/days", () => {
    const now = Date.now();
    expect(formatRelative(new Date(now - 10_000).toISOString(), "now")).toBe("now");
    expect(formatRelative(new Date(now - 5 * 60_000).toISOString(), "now")).toBe("5m");
    expect(formatRelative(new Date(now - 3 * 3600_000).toISOString(), "now")).toBe("3h");
    expect(formatRelative(new Date(now - 2 * 86400_000).toISOString(), "now")).toBe("2d");
  });
});

describe("qualityStatus", () => {
  it("is insufficient below the sample-size floor", () => {
    expect(qualityStatus(0, 0)).toBe("insufficient");
    expect(qualityStatus(0.5, 49)).toBe("insufficient");
  });

  it("buckets the ratio once enough samples exist", () => {
    expect(qualityStatus(0.005, 100)).toBe("excellent");
    expect(qualityStatus(0.02, 100)).toBe("good");
    expect(qualityStatus(0.04, 100)).toBe("fair");
    expect(qualityStatus(0.08, 100)).toBe("marginal");
    expect(qualityStatus(0.2, 100)).toBe("poor");
  });
});
