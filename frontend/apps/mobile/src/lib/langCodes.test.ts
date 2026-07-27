import { describe, it, expect } from "vitest";
import { normalizeLangCode } from "./langCodes";

describe("normalizeLangCode", () => {
  it("does not confuse Slovak (slo, 639-2/B) with Slovenian's real code (sl)", () => {
    // A naive slice(0,2) turns "slo" into "sl", which is Slovenian's actual
    // 639-1 code - the wrong language would be silently selected.
    expect(normalizeLangCode("slo")).toBe("sk");
    expect(normalizeLangCode("slk")).toBe("sk"); // 639-2/T form
    expect(normalizeLangCode("slv")).toBe("sl"); // Slovenian stays Slovenian
  });

  it("maps German 639-2 codes to 'de', not a naive 'ge'", () => {
    expect(normalizeLangCode("ger")).toBe("de");
    expect(normalizeLangCode("deu")).toBe("de");
  });

  it("maps French bibliographic/terminological forms to 'fr'", () => {
    expect(normalizeLangCode("fre")).toBe("fr");
    expect(normalizeLangCode("fra")).toBe("fr");
  });

  it("strips a BCP-47 region subtag before mapping", () => {
    expect(normalizeLangCode("fr-FR")).toBe("fr");
    expect(normalizeLangCode("FR-fr")).toBe("fr");
  });

  it("falls back to a 2-letter slice for codes outside the table", () => {
    expect(normalizeLangCode("xyz")).toBe("xy");
  });

  it("is empty-safe", () => {
    expect(normalizeLangCode("")).toBe("");
  });
});
