import { describe, it, expect } from "vitest";
import { formatAudioTrackLabel, formatChannelLayout, trackDisplayName } from "./audioChannelLayout";

describe("formatChannelLayout", () => {
  // The drift this replaces: web's track menu said "2.0", web's own stream
  // info panel said "Stereo", for the same track on the same screen.
  it("uses one notation family for every known layout", () => {
    expect(formatChannelLayout(1)).toBe("1.0");
    expect(formatChannelLayout(2)).toBe("2.0");
    expect(formatChannelLayout(6)).toBe("5.1");
    expect(formatChannelLayout(8)).toBe("7.1");
  });

  // Locale-neutral on purpose: the app ships in six languages and these
  // strings never go through i18n.
  it("never emits an English word", () => {
    for (const ch of [1, 2, 3, 4, 6, 7, 8, 12]) {
      expect(formatChannelLayout(ch)).not.toMatch(/mono|stereo|surround/i);
    }
  });

  it("degrades an unknown count instead of guessing a layout", () => {
    expect(formatChannelLayout(3)).toBe("3ch");
    expect(formatChannelLayout(7)).toBe("7ch");
  });

  it("gives the caller an empty string to skip the row on", () => {
    expect(formatChannelLayout(undefined)).toBe("");
    expect(formatChannelLayout(0)).toBe("");
    expect(formatChannelLayout(-2)).toBe("");
  });
});

describe("formatAudioTrackLabel", () => {
  it("prefers the title, then the language, then the caller's fallback", () => {
    expect(formatAudioTrackLabel({ title: "VF", language: "fra" }, "Audio")).toBe("VF");
    expect(formatAudioTrackLabel({ language: "fra" }, "Audio")).toBe("fra");
    expect(formatAudioTrackLabel({}, "Audio")).toBe("Audio");
  });

  it("appends the layout so two tracks differing only by it stay distinguishable", () => {
    expect(formatAudioTrackLabel({ title: "VF", channels: 6 }, "Audio")).toBe("VF - 5.1");
    expect(formatAudioTrackLabel({ title: "VF", channels: 2 }, "Audio")).toBe("VF - 2.0");
  });

  it("leaves the name alone when the channel count is unknown", () => {
    expect(formatAudioTrackLabel({ title: "VF" }, "Audio")).toBe("VF");
    expect(formatAudioTrackLabel({ title: "VF", channels: 0 }, "Audio")).toBe("VF");
  });
});

// The server sends an absent title as an empty string, so the old `??` chain
// rendered a blank entry in every track menu.
describe("trackDisplayName", () => {
  it("falls through an empty title to the language, then to the fallback", () => {
    expect(trackDisplayName({ title: "", language: "fra" }, "Track 4")).toBe("fra");
    expect(trackDisplayName({ title: "  ", language: "" }, "Track 4")).toBe("Track 4");
    expect(trackDisplayName({ title: "Commentary", language: "eng" }, "Track 4")).toBe("Commentary");
  });

  it("keeps an audio label readable when only the language is known", () => {
    expect(formatAudioTrackLabel({ title: "", language: "eng", channels: 2 }, "Track 1")).toBe("eng - 2.0");
  });
});
