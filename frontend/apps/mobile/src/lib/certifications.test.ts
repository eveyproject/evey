import { describe, it, expect } from "vitest";
import { certForLocale, certInfoForLocale } from "./certifications";

describe("certInfoForLocale", () => {
  const certs = { US: "PG-13", FR: "12", GB: "15", DE: "16" };

  it("prefers preferredCountry when it has a cert (highest precedence)", () => {
    expect(certInfoForLocale(certs, "en-US", undefined, "FR")).toEqual({ country: "FR", rating: "12" });
  });

  it("ignores preferredCountry when it has no cert, falling through to the locale", () => {
    // preferred JP not in certs → locale en-GB → GB
    expect(certInfoForLocale(certs, "en-GB", undefined, "JP")).toEqual({ country: "GB", rating: "15" });
  });

  it("uses the locale's region subtag when present", () => {
    expect(certInfoForLocale(certs, "en-GB", undefined)).toEqual({ country: "GB", rating: "15" });
  });

  it("derives the country from a bare language locale via the defaults map", () => {
    // "fr" → FR, "de" → DE
    expect(certInfoForLocale(certs, "fr", undefined)).toEqual({ country: "FR", rating: "12" });
    expect(certInfoForLocale(certs, "de", undefined)).toEqual({ country: "DE", rating: "16" });
  });

  it("falls back to US when neither preferred nor locale country matches", () => {
    expect(certInfoForLocale(certs, "ja-JP", undefined)).toEqual({ country: "US", rating: "PG-13" });
  });

  it("falls back to the alphabetically-first country when there is no US cert", () => {
    const noUS = { FR: "12", DE: "16", GB: "15" }; // sorted: DE, FR, GB → DE
    expect(certInfoForLocale(noUS, "ja-JP", undefined)).toEqual({ country: "DE", rating: "16" });
  });

  it("returns the fallback rating (preferredCountry || US) when certs is empty", () => {
    expect(certInfoForLocale({}, "fr-FR", "U")).toEqual({ country: "US", rating: "U" });
    expect(certInfoForLocale(undefined, "fr-FR", "U", "FR")).toEqual({ country: "FR", rating: "U" });
  });

  it("returns undefined when certs is empty and there is no fallback", () => {
    expect(certInfoForLocale({}, "fr-FR")).toBeUndefined();
    expect(certInfoForLocale(undefined, undefined)).toBeUndefined();
  });
});

describe("certForLocale", () => {
  it("returns just the resolved rating string", () => {
    expect(certForLocale({ FR: "12", US: "PG-13" }, "fr-FR")).toBe("12");
  });

  it("returns undefined when no cert resolves", () => {
    expect(certForLocale({}, "fr-FR")).toBeUndefined();
  });
});
