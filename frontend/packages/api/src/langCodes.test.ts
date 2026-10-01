import { describe, it, expect } from "vitest";
import { normalizeLangCode } from "./langCodes";

// The table exists because slice(0,2) is wrong for some 639-2 codes and
// COLLIDES for others; these pin exactly those cases.
describe("normalizeLangCode", () => {
  it("maps both bibliographic and terminologic 639-2 codes to one 639-1 code", () => {
    expect(normalizeLangCode("fre")).toBe("fr");
    expect(normalizeLangCode("fra")).toBe("fr");
    expect(normalizeLangCode("ger")).toBe("de");
    expect(normalizeLangCode("deu")).toBe("de");
    expect(normalizeLangCode("dut")).toBe("nl");
    expect(normalizeLangCode("nld")).toBe("nl");
  });

  it("never falls back to a slice that would name another language", () => {
    expect(normalizeLangCode("por")).toBe("pt"); // not "po"
    expect(normalizeLangCode("slo")).toBe("sk"); // not "sl", which is Slovenian
    expect(normalizeLangCode("slv")).toBe("sl");
    expect(normalizeLangCode("chi")).toBe("zh");
    expect(normalizeLangCode("jpn")).toBe("ja");
  });

  it("strips a region or script subtag and normalises case and space", () => {
    expect(normalizeLangCode("fr-FR")).toBe("fr");
    expect(normalizeLangCode("pt-BR")).toBe("pt");
    expect(normalizeLangCode(" EN-us ")).toBe("en");
    expect(normalizeLangCode("FRE")).toBe("fr");
    expect(normalizeLangCode("zh-Hant-TW")).toBe("zh");
  });

  it("keeps 2-letter codes and 3-letter codes it does not know as a best guess", () => {
    expect(normalizeLangCode("fr")).toBe("fr");
    expect(normalizeLangCode("fil")).toBe("fil");
    expect(normalizeLangCode("xyz")).toBe("xy");
  });

  it("is empty for nothing", () => {
    expect(normalizeLangCode(undefined)).toBe("");
    expect(normalizeLangCode("")).toBe("");
    expect(normalizeLangCode("  ")).toBe("");
    expect(normalizeLangCode("-FR")).toBe("");
  });
});

describe("languageDisplayName", () => {
  it("names the 639-2 codes containers carry, in the UI language", async () => {
    const { languageDisplayName } = await import("./langCodes");
    expect(languageDisplayName("ger", "en")).toBe("German");
    expect(languageDisplayName("fre", "en")).toBe("French");
    expect(languageDisplayName("eng", "en")).toBe("English");
    expect(languageDisplayName("dut", "en")).toBe("Dutch");
    // French UI: Intl says "allemand", the chip reads "Allemand".
    expect(languageDisplayName("ger", "fr")).toBe("Allemand");
    expect(languageDisplayName("pt-BR", "en")).toBe("Brazilian Portuguese");
  });

  it("names no language for undetermined tags and keeps an unknown code as written", async () => {
    const { languageDisplayName } = await import("./langCodes");
    expect(languageDisplayName("und", "en")).toBeNull();
    expect(languageDisplayName("", "en")).toBeNull();
    expect(languageDisplayName(undefined, "en")).toBeNull();
    expect(languageDisplayName("qqq", "en")).toBe("qqq");
  });
});
