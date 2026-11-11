import { describe, it, expect } from "vitest";
import { canonicalAudioCodec, declaresAudioCodec } from "./audioCodec";

describe("canonicalAudioCodec", () => {
  it("folds the aliases the server folds", () => {
    expect(canonicalAudioCodec("DCA")).toBe("dts");
    expect(canonicalAudioCodec("ec-3")).toBe("eac3");
    expect(canonicalAudioCodec("E-AC-3")).toBe("eac3");
    expect(canonicalAudioCodec("ac-3")).toBe("ac3");
    expect(canonicalAudioCodec("mlp")).toBe("truehd");
    expect(canonicalAudioCodec(" AAC ")).toBe("aac");
  });
});

describe("declaresAudioCodec", () => {
  const phone = ["aac", "mp3", "opus", "flac", "vorbis", "ac3", "eac3"];
  it("matches a declared codec under any of its names", () => {
    expect(declaresAudioCodec(phone, "ac-3")).toBe(true);
    expect(declaresAudioCodec(["e-ac-3"], "eac3")).toBe(true);
  });
  it("refuses what the device never declared", () => {
    expect(declaresAudioCodec(phone, "dts")).toBe(false);
    expect(declaresAudioCodec(phone, "dca")).toBe(false);
    expect(declaresAudioCodec(phone, "truehd")).toBe(false);
    expect(declaresAudioCodec([], "aac")).toBe(false);
  });
});
