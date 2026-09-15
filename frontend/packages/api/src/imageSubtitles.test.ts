import { describe, it, expect } from "vitest";
import { isBurnInSubtitle, isClientDrawableSubtitle, isImageSubtitle } from "./imageSubtitles";

// Three questions a player asks about a subtitle codec, and the one case that
// separates them: PGS is a bitmap the player draws itself, so it is an image
// but not a burn-in.
describe("subtitle codec classification", () => {
  it("knows both the probe spelling and the encoder alias of each bitmap format", () => {
    for (const codec of ["hdmv_pgs_subtitle", "pgssub", "dvd_subtitle", "dvdsub", "dvb_subtitle", "dvbsub", "xsub"]) {
      expect(isImageSubtitle(codec), codec).toBe(true);
    }
    for (const codec of ["subrip", "ass", "webvtt", "mov_text", "", undefined, null]) {
      expect(isImageSubtitle(codec), String(codec)).toBe(false);
    }
  });

  it("tolerates case and stray space, the value comes from a probe", () => {
    expect(isImageSubtitle(" HDMV_PGS_SUBTITLE ")).toBe(true);
    expect(isClientDrawableSubtitle("PGS")).toBe(true);
  });

  it("only PGS is drawable client-side; the other bitmaps still burn in", () => {
    expect(isClientDrawableSubtitle("hdmv_pgs_subtitle")).toBe(true);
    expect(isClientDrawableSubtitle("pgssub")).toBe(true);
    expect(isBurnInSubtitle("hdmv_pgs_subtitle")).toBe(false);

    for (const codec of ["dvd_subtitle", "dvdsub", "dvb_subtitle", "dvbsub", "xsub"]) {
      expect(isClientDrawableSubtitle(codec), codec).toBe(false);
      expect(isBurnInSubtitle(codec), codec).toBe(true);
    }
    // Text is neither.
    expect(isClientDrawableSubtitle("subrip")).toBe(false);
    expect(isBurnInSubtitle("subrip")).toBe(false);
  });
});
