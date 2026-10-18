import { describe, expect, it } from "vitest";
import { libraryFilterOptions, normalizeCodec } from "./libraryFilterFacets";

describe("libraryFilterOptions", () => {
  it("names each resolution tier with the poster badge's band, in band order", () => {
    expect(libraryFilterOptions({ resolutionTiers: [0, 1080, 2160] }).resolutions).toEqual(["4K", "1080p", "SD"]);
    expect(libraryFilterOptions({ resolutionTiers: [1440, 720] }).resolutions).toEqual(["1440p", "720p"]);
  });

  it("labels codecs the way the chips match them, one chip per label", () => {
    expect(libraryFilterOptions({ videoCodecs: ["hevc", "h265", "h264", "mjpeg"] }).codecs).toEqual([
      "H.264",
      "H.265/HEVC",
      "MJPEG",
    ]);
    expect(normalizeCodec("avc")).toBe("H.264");
  });

  it("folds the 639-2/B and 639-2/T forms of a language into one chip and drops und", () => {
    const options = libraryFilterOptions({
      audioLanguages: ["fre", "fra", "und", "", "jpn"],
      subtitleLanguages: ["ger", "deu"],
    });
    expect(options.audioLangs).toEqual(["fr", "ja"]);
    expect(options.subLangs).toEqual(["de"]);
  });

  it("offers nothing before the facets arrive", () => {
    expect(libraryFilterOptions(undefined)).toEqual({ resolutions: [], codecs: [], audioLangs: [], subLangs: [] });
  });
});
