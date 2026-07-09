import { describe, it, expect } from "vitest";
import { LABELS, translate } from "../src/i18n/labels";

describe("labels", () => {
  it("every key has all four locales, non-empty", () => {
    for (const [key, dict] of Object.entries(LABELS)) {
      for (const loc of ["en", "fr", "es", "de"] as const) {
        expect(dict[loc], `${key}.${loc}`).toBeTruthy();
      }
    }
  });

  it("translate falls back to en, then key", () => {
    expect(translate("set_save", "fr")).toBe("Enregistrer");
    expect(translate("set_save", "pt")).toBe("Save");
    expect(translate("set_save", undefined)).toBe("Save");
    expect(translate("does_not_exist", "fr")).toBe("does_not_exist");
  });

  it("locale region variants resolve (fr-FR)", () => {
    expect(translate("set_speed", "fr-FR")).toBe("Vitesse");
  });
});
