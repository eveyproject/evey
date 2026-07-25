import { describe, it, expect } from "vitest";
import { pickNativeAudioTrack } from "./audioTrackMatch";

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
});
