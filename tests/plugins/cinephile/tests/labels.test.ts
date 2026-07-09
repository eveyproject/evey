import { describe, it, expect } from "vitest";
import { LABELS, translate } from "../src/i18n/labels";

const LOCALES = ["en", "fr", "es", "de"] as const;

describe("LABELS", () => {
  it("has a non-empty translation for every locale on every key", () => {
    for (const [key, dict] of Object.entries(LABELS)) {
      for (const locale of LOCALES) {
        expect(dict[locale], `${key}.${locale}`).toBeTruthy();
        expect(dict[locale].length).toBeGreaterThan(0);
      }
    }
  });
});

describe("translate", () => {
  it("returns the requested locale's string", () => {
    expect(translate("statTotal", "fr")).toBe(LABELS.statTotal.fr);
    expect(translate("statTotal", "en")).toBe(LABELS.statTotal.en);
  });

  it("matches on the first two characters of a longer locale tag", () => {
    expect(translate("statMovies", "fr-FR")).toBe(LABELS.statMovies.fr);
    expect(translate("statMovies", "en-US")).toBe(LABELS.statMovies.en);
  });

  it("falls back to English for an unsupported locale", () => {
    expect(translate("statSeries", "ja")).toBe(LABELS.statSeries.en);
  });

  it("falls back to English when no locale is given", () => {
    expect(translate("statSpan", undefined)).toBe(LABELS.statSpan.en);
  });

  it("returns the key itself for an unknown key", () => {
    expect(translate("nope", "en")).toBe("nope");
  });
});
