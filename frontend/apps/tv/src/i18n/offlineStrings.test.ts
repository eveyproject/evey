import { describe, it, expect } from "vitest";
import { OFFLINE_STRINGS } from "./offlineStrings";

const LANGS = ["en", "fr", "de", "es", "hi", "zh"];

describe("bundled pre-connect strings", () => {
  it("covers every language the app offers", () => {
    expect(Object.keys(OFFLINE_STRINGS).sort()).toEqual([...LANGS].sort());
  });

  // A language missing a key would send that one screen back to English, which
  // is the very failure these strings exist to prevent.
  it("carries the same keys in every language", () => {
    const reference = Object.keys(OFFLINE_STRINGS.en!).sort();
    for (const lang of LANGS) {
      expect(Object.keys(OFFLINE_STRINGS[lang]!).sort(), `${lang} diverges from en`).toEqual(reference);
    }
  });

  it("has no empty translation", () => {
    for (const [lang, strings] of Object.entries(OFFLINE_STRINGS)) {
      for (const [key, value] of Object.entries(strings)) {
        expect(value.trim().length, `${lang}.${key} is empty`).toBeGreaterThan(0);
      }
    }
  });

  // The point is a screen a viewer meets before any server answers.
  it("covers the server-connect screen and says why it is showing", () => {
    for (const key of [
      "mobile.serverConnect.title",
      "mobile.serverConnect.connect",
      "mobile.serverConnect.serverUnreachable",
      "mobile.servers.addAnother",
      "mobile.servers.delete",
    ]) {
      expect(OFFLINE_STRINGS.fr![key], `${key} missing`).toBeTruthy();
    }
  });

  it("is not translated to English by mistake", () => {
    expect(OFFLINE_STRINGS.fr!["mobile.servers.delete"]).not.toBe(OFFLINE_STRINGS.en!["mobile.servers.delete"]);
    expect(OFFLINE_STRINGS.fr!["mobile.serverConnect.title"]).not.toBe(OFFLINE_STRINGS.en!["mobile.serverConnect.title"]);
  });
});
