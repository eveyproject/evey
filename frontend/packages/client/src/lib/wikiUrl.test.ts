import { describe, it, expect } from "vitest";
import { wikiUrl } from "./wikiUrl";

describe("wikiUrl", () => {
  it("serves every published language under its prefix, English at the root", () => {
    expect(wikiUrl("fr", "privacy/")).toBe("https://eveyproject.org/fr/privacy/");
    expect(wikiUrl("de-DE", "privacy/")).toBe("https://eveyproject.org/de/privacy/");
    expect(wikiUrl("zh", "/getting-started/apps/")).toBe("https://eveyproject.org/zh/getting-started/apps/");
    expect(wikiUrl("en", "privacy/")).toBe("https://eveyproject.org/privacy/");
  });

  it("falls back to English for a language the wiki does not publish", () => {
    expect(wikiUrl("it", "privacy/")).toBe("https://eveyproject.org/privacy/");
    expect(wikiUrl(undefined, "privacy/")).toBe("https://eveyproject.org/privacy/");
  });
});
