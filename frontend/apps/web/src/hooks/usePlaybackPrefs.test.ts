import { describe, it, expect, beforeEach } from "vitest";

// localStorage is reset between tests via jsdom
describe("usePlaybackPrefs — audio/subtitle lang", () => {
  beforeEach(() => localStorage.clear());

  it("getPreferredAudioLang returns null when not set", async () => {
    const { getPreferredAudioLang } = await import("./usePlaybackPrefs");
    expect(getPreferredAudioLang()).toBeNull();
  });

  it("setPreferredAudioLang / getPreferredAudioLang round-trips", async () => {
    const { setPreferredAudioLang, getPreferredAudioLang } = await import("./usePlaybackPrefs");
    setPreferredAudioLang("fr");
    expect(getPreferredAudioLang()).toBe("fr");
  });

  it("setPreferredAudioLang(null) removes value", async () => {
    const { setPreferredAudioLang, getPreferredAudioLang } = await import("./usePlaybackPrefs");
    setPreferredAudioLang("en");
    setPreferredAudioLang(null);
    expect(getPreferredAudioLang()).toBeNull();
  });

  it("getPreferredSubtitleLang returns null when not set", async () => {
    const { getPreferredSubtitleLang } = await import("./usePlaybackPrefs");
    expect(getPreferredSubtitleLang()).toBeNull();
  });

  it("setPreferredSubtitleLang / getPreferredSubtitleLang round-trips", async () => {
    const { setPreferredSubtitleLang, getPreferredSubtitleLang } = await import("./usePlaybackPrefs");
    setPreferredSubtitleLang("es");
    expect(getPreferredSubtitleLang()).toBe("es");
  });
});

describe("usePlaybackPrefs — series track prefs", () => {
  beforeEach(() => localStorage.clear());

  it("getSeriesTrackPrefs returns null when not set", async () => {
    const { getSeriesTrackPrefs } = await import("./usePlaybackPrefs");
    expect(getSeriesTrackPrefs("series-1")).toBeNull();
  });

  it("setSeriesTrackPrefs / getSeriesTrackPrefs round-trips", async () => {
    const { setSeriesTrackPrefs, getSeriesTrackPrefs } = await import("./usePlaybackPrefs");
    setSeriesTrackPrefs("series-1", { audioLang: "fr", subtitleLang: "en" });
    expect(getSeriesTrackPrefs("series-1")).toEqual({ audioLang: "fr", subtitleLang: "en" });
  });

  it("isolates prefs per seriesId", async () => {
    const { setSeriesTrackPrefs, getSeriesTrackPrefs } = await import("./usePlaybackPrefs");
    setSeriesTrackPrefs("series-a", { audioLang: "de" });
    setSeriesTrackPrefs("series-b", { audioLang: "ja" });
    expect(getSeriesTrackPrefs("series-a")).toEqual({ audioLang: "de" });
    expect(getSeriesTrackPrefs("series-b")).toEqual({ audioLang: "ja" });
  });
});

describe("usePlaybackPrefs — subtitle style", () => {
  beforeEach(() => localStorage.clear());

  it("getSubtitleStyle returns DEFAULT_SUBTITLE_STYLE when not set", async () => {
    const { getSubtitleStyle, DEFAULT_SUBTITLE_STYLE } = await import("./usePlaybackPrefs");
    expect(getSubtitleStyle()).toEqual(DEFAULT_SUBTITLE_STYLE);
  });

  it("setSubtitleStyle / getSubtitleStyle round-trips", async () => {
    const { setSubtitleStyle, getSubtitleStyle, DEFAULT_SUBTITLE_STYLE } = await import("./usePlaybackPrefs");
    const custom = { ...DEFAULT_SUBTITLE_STYLE, fontSize: 150, color: "#ff0000" };
    setSubtitleStyle(custom);
    expect(getSubtitleStyle()).toEqual(custom);
  });

  it("getSubtitleStyle merges defaults for missing fields", async () => {
    const { getSubtitleStyle, DEFAULT_SUBTITLE_STYLE } = await import("./usePlaybackPrefs");
    // Manually set partial JSON to simulate an old/partial stored value
    localStorage.setItem("evey:subtitle-style", JSON.stringify({ fontSize: 200 }));
    const result = getSubtitleStyle();
    expect(result.fontSize).toBe(200);
    expect(result.color).toBe(DEFAULT_SUBTITLE_STYLE.color);
  });
});
