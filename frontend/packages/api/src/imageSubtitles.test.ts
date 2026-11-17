import { describe, it, expect } from "vitest";
import { isBurnInSubtitle, isClientDrawableSubtitle, isImageSubtitle } from "./imageSubtitles";

// Three questions a player asks about a subtitle codec, and the case that
// separates them: PGS, VobSub and DVB are bitmaps the player draws itself, so
// they are images but not burn-ins.
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

  it("PGS, VobSub and DVB are drawable client-side; XSUB still burns in", () => {
    for (const codec of ["hdmv_pgs_subtitle", "pgssub", "dvd_subtitle", "dvdsub", "dvb_subtitle", "dvbsub"]) {
      expect(isClientDrawableSubtitle(codec), codec).toBe(true);
      expect(isBurnInSubtitle(codec), codec).toBe(false);
    }
    expect(isClientDrawableSubtitle("xsub")).toBe(false);
    expect(isBurnInSubtitle("xsub")).toBe(true);
    // Text is neither.
    expect(isClientDrawableSubtitle("subrip")).toBe(false);
    expect(isBurnInSubtitle("subrip")).toBe(false);
  });
});
