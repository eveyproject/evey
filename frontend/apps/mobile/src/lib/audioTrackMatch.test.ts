import { describe, it, expect } from "vitest";
import { pickNativeAudioTrack, nativeTrackToApply, pickPreferredStreamIndex } from "./audioTrackMatch";

describe("pickNativeAudioTrack", () => {
  const tracks = [
    { id: "0", language: "fre", label: "VFF" },
    { id: "1", language: "eng", label: "ENG" },
  ];

  it("picks by 0-based position (source order matches)", () => {
    expect(pickNativeAudioTrack(tracks, 0, "fre")?.label).toBe("VFF");
    expect(pickNativeAudioTrack(tracks, 1, "eng")?.label).toBe("ENG");
  });

  it("falls back to language when the position is out of range", () => {
    // 639-2 "eng" must still match a native "en" (639-1).
    expect(pickNativeAudioTrack([{ language: "en", label: "English" }], 5, "eng")?.label).toBe("English");
  });

  it("returns null when the manifest tracks are not parsed yet", () => {
    expect(pickNativeAudioTrack([], 0, "fre")).toBeNull();
  });

  it("returns null when nothing matches so the caller restarts instead", () => {
    expect(pickNativeAudioTrack([{ language: "de", label: "Deutsch" }], 9, "fre")).toBeNull();
  });

  it("does not confuse Slovak (slo) with a Slovenian native track", () => {
    // A naive slice(0,2) normalization turns "slo" (Slovak) into "sl", which
    // collides with Slovenian's real 639-1 code - the wrong track would match.
    const slovenianOnly = [{ language: "slv", label: "Slovenian" }];
    expect(pickNativeAudioTrack(slovenianOnly, 9, "slo")).toBeNull();
  });

  it("matches German (ger/deu, 639-2) against a native 'de' (639-1) track", () => {
    // A naive slice(0,2) of "ger" gives "ge", which never matches "de".
    const germanTrack = [{ language: "de", label: "Deutsch" }];
    expect(pickNativeAudioTrack(germanTrack, 9, "ger")?.label).toBe("Deutsch");
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

  it("returns null when no preference matches anything in the file", () => {
    expect(pickPreferredStreamIndex(streams, ["de", "ko"])).toBeNull();
  });

  it("returns null for an empty preference list", () => {
    expect(pickPreferredStreamIndex(streams, [])).toBeNull();
  });
});

// A file played directly opens on ITS default track: the title page's pick
// showed as selected in the menu while the other language played (tablet,
// 2026-10-03).
describe("nativeTrackToApply", () => {
  const tracks = [
    { id: "0", language: "fre", label: "VFF" },
    { id: "1", language: "eng", label: "ENG" },
  ];

  it("switches to the picked stream when the player opened on another", () => {
    expect(nativeTrackToApply(tracks, tracks[0], 1, "eng")?.label).toBe("ENG");
  });

  it("leaves the player alone when it already plays the picked stream", () => {
    expect(nativeTrackToApply(tracks, tracks[1], 1, "eng")).toBeNull();
  });

  it("applies the pick when the player does not say what it plays", () => {
    expect(nativeTrackToApply(tracks, null, 1, "eng")?.label).toBe("ENG");
  });

  it("waits for the native tracks", () => {
    expect(nativeTrackToApply([], null, 1, "eng")).toBeNull();
  });
});
