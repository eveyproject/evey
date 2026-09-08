import { describe, it, expect } from "vitest";
import { AUTOPLAY_NEXT_PREF_KEY, autoplayNextEpisode, autoplayNextPreferences } from "./autoplayNext";

describe("autoplayNextEpisode", () => {
  it("is on by default: a profile that never chose keeps the historical behaviour", () => {
    expect(autoplayNextEpisode(undefined)).toBe(true);
    expect(autoplayNextEpisode({})).toBe(true);
    expect(autoplayNextEpisode({ theme: "dark" })).toBe(true);
  });

  it("is off only on an explicit 'false'", () => {
    expect(autoplayNextEpisode({ [AUTOPLAY_NEXT_PREF_KEY]: "false" })).toBe(false);
    expect(autoplayNextEpisode({ [AUTOPLAY_NEXT_PREF_KEY]: "true" })).toBe(true);
  });
});

describe("autoplayNextPreferences", () => {
  it("merges onto the existing map: the server replaces the whole map", () => {
    const next = autoplayNextPreferences({ audio_languages: "fr", theme: "dark" }, false);
    expect(next).toEqual({ audio_languages: "fr", theme: "dark", [AUTOPLAY_NEXT_PREF_KEY]: "false" });
  });

  it("does not mutate the input", () => {
    const current = { theme: "dark" };
    autoplayNextPreferences(current, true);
    expect(current).toEqual({ theme: "dark" });
  });
});
