import { describe, it, expect } from "vitest";
import { formatAudioTrackLabel, formatChannelLayout } from "./audioTrackLabel";

describe("formatChannelLayout", () => {
  it("nomme les dispositions courantes", () => {
    expect(formatChannelLayout(1)).toBe("Mono");
    expect(formatChannelLayout(2)).toBe("Stereo");
    expect(formatChannelLayout(6)).toBe("5.1");
    expect(formatChannelLayout(8)).toBe("7.1");
  });

  it("se rabat sur le compte brut pour les cas sans nom", () => {
    expect(formatChannelLayout(3)).toBe("3ch");
  });

  it("ne dit rien quand le nombre de canaux est absent ou absurde", () => {
    expect(formatChannelLayout(undefined)).toBe("");
    expect(formatChannelLayout(0)).toBe("");
    expect(formatChannelLayout(-2)).toBe("");
  });
});

describe("formatAudioTrackLabel", () => {
  // The point of the fix: two French tracks differing only by layout were
  // rendered identically in the mobile menu, so there was no way to pick one.
  it("distingue deux pistes qui ne different que par la disposition", () => {
    const stereo = formatAudioTrackLabel({ language: "fra", channels: 2 }, "Piste 1");
    const surround = formatAudioTrackLabel({ language: "fra", channels: 6 }, "Piste 1");
    expect(stereo).toBe("fra - Stereo");
    expect(surround).toBe("fra - 5.1");
    expect(stereo).not.toBe(surround);
  });

  it("prefere le titre a la langue", () => {
    expect(formatAudioTrackLabel({ title: "Commentary", language: "eng" }, "Piste 1")).toBe("Commentary");
  });

  it("utilise le repli de l'appelant quand la piste n'a ni titre ni langue", () => {
    expect(formatAudioTrackLabel({}, "Piste 1")).toBe("Piste 1");
    expect(formatAudioTrackLabel({ channels: 6 }, "Piste 1")).toBe("Piste 1 - 5.1");
  });
});
