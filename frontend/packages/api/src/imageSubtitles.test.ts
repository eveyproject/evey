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

  // The server decodes bitmaps out of Matroska only: a DVB track of an MPEG-TS
  // recording, or a VobSub track of an MPEG-PS or AVI, has no blob to fetch and
  // must stay a burn-in, or it is selectable and shows nothing.
  it("draws a bitmap client-side only out of a container the server decodes it from", () => {
    for (const container of ["matroska", "Matroska", "matroska,webm", "webm", "", undefined, null]) {
      expect(isClientDrawableSubtitle("dvb_subtitle", container), String(container)).toBe(true);
      expect(isBurnInSubtitle("dvb_subtitle", container), String(container)).toBe(false);
    }
    for (const container of ["mpegts", "avi", "mp4", "mpeg"]) {
      for (const codec of ["dvb_subtitle", "dvd_subtitle", "hdmv_pgs_subtitle"]) {
        expect(isClientDrawableSubtitle(codec, container), `${codec} in ${container}`).toBe(false);
        expect(isBurnInSubtitle(codec, container), `${codec} in ${container}`).toBe(true);
      }
    }
    expect(isBurnInSubtitle("subrip", "mpegts")).toBe(false);
  });
});
