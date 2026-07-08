import { describe, it, expect } from "vitest";
import { LABELS, translate } from "../src/i18n/labels";
import { CONSTELLATIONS } from "../src/domain/data/constellations";

describe("labels", () => {
  it("has fr/en/es/de for every key", () => {
    for (const [key, dict] of Object.entries(LABELS)) {
      for (const lang of ["fr", "en", "es", "de"] as const) {
        expect(dict[lang], `${key}.${lang}`).toBeTruthy();
      }
    }
  });

  it("has a label for every constellation used in the data set", () => {
    for (const con of CONSTELLATIONS) {
      expect(LABELS[con.labelKey], `missing label for ${con.labelKey}`).toBeTruthy();
    }
  });

  it("falls back to English for an unknown locale, and to the key itself for an unknown key", () => {
    expect(translate("con_orion", "zz")).toBe(LABELS.con_orion.en);
    expect(translate("does_not_exist", "fr")).toBe("does_not_exist");
  });

  it("translate() picks the right language", () => {
    expect(translate("con_orion", "fr")).toBe("Orion");
    expect(translate("planet_saturn", "fr")).toBe("Saturne");
  });
});
