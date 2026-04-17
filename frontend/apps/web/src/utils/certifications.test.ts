import { describe, it, expect } from "vitest";
import { certForLocale, certInfoForLocale } from "./certifications";

describe("certForLocale", () => {
  it("returns fallback when certs empty", () => {
    expect(certForLocale({}, "fr", "R")).toBe("R");
    expect(certForLocale(undefined, "fr", "R")).toBe("R");
  });

  it("returns undefined when nothing", () => {
    expect(certForLocale({}, "fr")).toBeUndefined();
    expect(certForLocale(undefined, undefined)).toBeUndefined();
  });

  it("picks exact country from locale region", () => {
    const certs = { US: "PG-13", FR: "-12", GB: "12" };
    expect(certForLocale(certs, "en-GB")).toBe("12");
    expect(certForLocale(certs, "fr-FR")).toBe("-12");
  });

  it("maps bare language to default country", () => {
    const certs = { US: "PG-13", FR: "-12", DE: "12", IN: "UA" };
    expect(certForLocale(certs, "fr")).toBe("-12");
    expect(certForLocale(certs, "de")).toBe("12");
    expect(certForLocale(certs, "en")).toBe("PG-13");
  });

  it("falls back to US when country unknown", () => {
    const certs = { US: "PG-13", JP: "G" };
    expect(certForLocale(certs, "zz")).toBe("PG-13");
  });

  it("falls back to first entry alphabetically when neither country nor US", () => {
    const certs = { JP: "G", KR: "12" };
    expect(certForLocale(certs, "fr")).toBe("G");
  });

  it("accepts underscore-separated locales", () => {
    const certs = { FR: "-12", US: "PG-13" };
    expect(certForLocale(certs, "fr_FR")).toBe("-12");
  });
});

describe("certInfoForLocale", () => {
  it("returns country + rating for matched locale", () => {
    expect(certInfoForLocale({ FR: "-12", US: "PG-13" }, "fr-FR")).toEqual({ country: "FR", rating: "-12" });
  });

  it("falls back to US", () => {
    expect(certInfoForLocale({ US: "PG-13" }, "zz")).toEqual({ country: "US", rating: "PG-13" });
  });

  it("uses fallback rating when certs empty", () => {
    expect(certInfoForLocale({}, "fr", "R")).toEqual({ country: "US", rating: "R" });
  });

  it("returns undefined when nothing", () => {
    expect(certInfoForLocale({}, "fr")).toBeUndefined();
  });
});
