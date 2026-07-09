import { describe, it, expect } from "vitest";
import { translate, LABELS } from "../src/i18n/labels";

describe("translate", () => {
  it("returns the French label for locale 'fr'", () => {
    expect(translate("cta", "fr")).toBe("Ce soir, on regarde ?");
  });

  it("returns the English label for locale 'en-US'", () => {
    expect(translate("cta", "en-US")).toBe(LABELS.cta.en);
  });

  it("falls back to English for an unknown locale", () => {
    expect(translate("cta", "xx")).toBe(LABELS.cta.en);
  });

  it("falls back to English when locale is undefined", () => {
    expect(translate("cta", undefined)).toBe(LABELS.cta.en);
  });

  it("returns the key itself for an unknown key", () => {
    expect(translate("no_such_key", "fr")).toBe("no_such_key");
  });

  it("has every declared locale populated for every key", () => {
    const locales = ["en", "fr", "es", "de"] as const;
    for (const key of Object.keys(LABELS)) {
      for (const locale of locales) {
        expect(LABELS[key][locale]).toBeTruthy();
      }
    }
  });
});
