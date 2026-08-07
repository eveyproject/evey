import { describe, it, expect } from "vitest";
import { defaultRegionForLocale } from "./regionDefaults";

describe("defaultRegionForLocale", () => {
  it("falls back to GB when the locale is missing", () => {
    expect(defaultRegionForLocale(undefined)).toBe("GB");
  });

  it("extracts the region subtag when the locale already carries one", () => {
    expect(defaultRegionForLocale("fr-CA")).toBe("CA");
    expect(defaultRegionForLocale("en_US")).toBe("US");
  });

  it("maps a bare language code to its known default region", () => {
    expect(defaultRegionForLocale("fr")).toBe("FR");
    expect(defaultRegionForLocale("de")).toBe("DE");
    expect(defaultRegionForLocale("es")).toBe("ES");
    expect(defaultRegionForLocale("hi")).toBe("IN");
    expect(defaultRegionForLocale("zh")).toBe("CN");
    expect(defaultRegionForLocale("en")).toBe("GB");
  });

  it("falls back to GB for an unmapped bare language code", () => {
    expect(defaultRegionForLocale("ja")).toBe("GB");
  });
});
