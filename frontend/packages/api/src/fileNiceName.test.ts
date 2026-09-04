import { describe, expect, it } from "vitest";
import { niceNameFromPath } from "./fileNiceName";

describe("niceNameFromPath", () => {
  it("strips the quality tag the user asked about", () => {
    expect(niceNameFromPath("/media/films/Rocket 2 - 1080P.mkv")).toBe("Rocket 2");
  });

  it("handles the real library's naming", () => {
    expect(niceNameFromPath("/media/films/Arrival - 1080p.mkv")).toBe("Arrival");
    expect(niceNameFromPath("/media/Avatar - 2160P.mkv")).toBe("Avatar");
    expect(niceNameFromPath("/media/dvdrip/L'agence.avi")).toBe("L'agence");
  });

  it("strips codec, audio, source and language tags", () => {
    expect(niceNameFromPath("Some Movie.2020.1080p.BluRay.x264.AC3-GROUP.mkv")).toBe("Some Movie 2020");
    expect(niceNameFromPath("Some Movie MULTI VFF DTS-HD 2160p HEVC.mkv")).toBe("Some Movie");
    expect(niceNameFromPath("Some Movie [1080p] {WEB-DL} VOSTFR.mp4")).toBe("Some Movie");
  });

  it("strips episode markers", () => {
    expect(niceNameFromPath("/media/series/Some Series/S03/Some Series S03E10 - MULTI - 1080P.mkv")).toBe("Some Series");
    expect(niceNameFromPath("Some Series 2x07 720p.mkv")).toBe("Some Series");
  });

  // The failure mode that matters: over-stripping. A suggestion that eats part
  // of the title is worse than one that leaves a tag behind, because the admin
  // cannot tell what was removed.
  it("never eats digits or words that belong to the title", () => {
    expect(niceNameFromPath("Rocket 2.mkv")).toBe("Rocket 2");
    expect(niceNameFromPath("Se7en.mkv")).toBe("Se7en");
    expect(niceNameFromPath("Spider-Man.mkv")).toBe("Spider-Man");
    expect(niceNameFromPath("Ocean's Eleven.mkv")).toBe("Ocean's Eleven");
    expect(niceNameFromPath("21 Jump Street.mkv")).toBe("21 Jump Street");
  });

  it("keeps a title that is only a year", () => {
    expect(niceNameFromPath("1917.mkv")).toBe("1917");
    expect(niceNameFromPath("/media/films/2012 - 1080p.mkv")).toBe("2012");
  });

  it("handles dotted and underscored release names", () => {
    expect(niceNameFromPath("The.Big.Movie.2019.720p.WEBRip.mkv")).toBe("The Big Movie 2019");
    expect(niceNameFromPath("The_Big_Movie_1080p.mkv")).toBe("The Big Movie");
  });

  it("returns empty for nothing usable, so callers can fall back", () => {
    expect(niceNameFromPath("")).toBe("");
    expect(niceNameFromPath(null)).toBe("");
    expect(niceNameFromPath(undefined)).toBe("");
    expect(niceNameFromPath("/media/1080p.mkv")).toBe("");
  });

  it("accepts Windows separators", () => {
    expect(niceNameFromPath("D:\\media\\films\\Rocket 2 - 1080P.mkv")).toBe("Rocket 2");
  });
});
