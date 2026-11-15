import { describe, it, expect } from "vitest";
import { DEFAULT_SUBTITLE_STYLE, parseSubtitleStylePref, subtitleStylePreferences } from "./subtitleStyle";

describe("the subtitle appearance stored on a profile", () => {
  it("round-trips through the preferences map without touching the other keys", () => {
    const style = { ...DEFAULT_SUBTITLE_STYLE, fontSize: 140, color: "#fde047", position: "top" as const, shadow: true };
    const prefs = subtitleStylePreferences({ theme: "amoled", audio_languages: "fr" }, style);
    expect(prefs.theme).toBe("amoled");
    expect(prefs.audio_languages).toBe("fr");
    expect(parseSubtitleStylePref(prefs)).toEqual(style);
  });

  it("is null for a profile that never chose, or a value that is not an object", () => {
    expect(parseSubtitleStylePref(undefined)).toBeNull();
    expect(parseSubtitleStylePref({ theme: "amoled" })).toBeNull();
    expect(parseSubtitleStylePref({ subtitle_style: "not json" })).toBeNull();
    expect(parseSubtitleStylePref({ subtitle_style: "42" })).toBeNull();
  });

  // Written by another client, possibly an older or a newer one: a field it
  // got wrong must not reach the renderer, and a field it added is ignored.
  it("clamps what is out of range and defaults what is not understood", () => {
    const parsed = parseSubtitleStylePref({
      subtitle_style: JSON.stringify({
        fontSize: 900, color: "red", bgColor: "#1e3a5f", bgOpacity: -5, position: "left", shadow: "yes", fontFamily: "serif",
      }),
    });
    expect(parsed).toEqual({
      fontSize: 200, color: "#ffffff", bgColor: "#1e3a5f", bgOpacity: 0, position: "bottom", shadow: false,
    });
  });
});
