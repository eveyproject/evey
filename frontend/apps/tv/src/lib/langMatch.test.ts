import { describe, it, expect } from "vitest";
import { normalizeLangCode, pickPreferredStreamIndex } from "./langMatch";

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
    expect(normalizeLangCode(undefined)).toBe("");
    expect(normalizeLangCode("")).toBe("");
  });
});

describe("pickPreferredStreamIndex", () => {
  const streams = [
    { index: 0, language: "eng" },
    { index: 1, language: "fre" },
    { index: 2, language: "jpn" },
  ];

  it("returns the index of the first preference found in the streams", () => {
    expect(pickPreferredStreamIndex(streams, ["ja", "fr"])).toBe(2);
  });

  it("skips a preferred language absent from the file and tries the next", () => {
    expect(pickPreferredStreamIndex(streams, ["de", "fr"])).toBe(1);
  });

  it("is 639-2/639-1 aware (German preference matches a 'ger' stream)", () => {
    expect(pickPreferredStreamIndex([{ index: 0, language: "ger" }], ["de"])).toBe(0);
  });

  it("does not confuse a Slovak preference with a Slovenian-only stream", () => {
    expect(pickPreferredStreamIndex([{ index: 0, language: "slv" }], ["sk"])).toBeNull();
  });

  it("returns null when no preference matches anything in the file", () => {
    expect(pickPreferredStreamIndex(streams, ["de", "ko"])).toBeNull();
  });

  it("returns null for an empty preference list", () => {
    expect(pickPreferredStreamIndex(streams, [])).toBeNull();
  });
});
