import { describe, it, expect } from "vitest";
import {
  audioDelayed,
  baseName,
  repairableCount,
  volumeBackupsRisky,
  volumeTempRisky,
} from "./seekIndexHelpers";

describe("audioDelayed", () => {
  it("is false when the delay is at or below the defect floor", () => {
    expect(audioDelayed({ audioDelaySec: 0 })).toBe(false);
    expect(audioDelayed({ audioDelaySec: 0.1 })).toBe(false);
    expect(audioDelayed({})).toBe(false);
  });

  it("is true above the defect floor", () => {
    expect(audioDelayed({ audioDelaySec: 0.11 })).toBe(true);
    expect(audioDelayed({ audioDelaySec: 1.5 })).toBe(true);
  });
});

describe("baseName", () => {
  it("strips the directory portion of a path", () => {
    expect(baseName("/data/media/movies/film.mkv")).toBe("film.mkv");
  });

  it("returns the input unchanged when there is no slash", () => {
    expect(baseName("film.mkv")).toBe("film.mkv");
  });
});

describe("repairableCount", () => {
  // Reproduces a prod bug already fixed on web: "repair all" must exclude
  // files no repair can help (truncated = tail gone, seekCoarse = already
  // rebuilt and still too sparse) from missingIndex, or the button promises
  // a fix it then silently skips.
  it("excludes truncated and seekCoarse files from the missing-index count", () => {
    const items = [
      { hasIndex: false, truncated: false, seekCoarse: false }, // repairable
      { hasIndex: false, truncated: true, seekCoarse: false }, // truncated - not repairable
      { hasIndex: false, truncated: false, seekCoarse: true }, // coarse - not repairable
      { hasIndex: true, truncated: false, seekCoarse: false }, // already indexed
      { hasIndex: false, truncated: false, seekCoarse: false }, // repairable
    ];
    expect(repairableCount(items)).toBe(2);
  });

  it("is 0 for an empty list", () => {
    expect(repairableCount([])).toBe(0);
  });

  it("treats missing hasIndex as un-indexed (counts it when repairable)", () => {
    expect(repairableCount([{}])).toBe(1);
  });
});

describe("volumeBackupsRisky", () => {
  it("is true when missing bytes plus headroom would overflow free space", () => {
    expect(volumeBackupsRisky({ missingBytes: String(900 * 1024 * 1024), freeBytes: String(1024 * 1024 * 1024) })).toBe(true);
  });

  it("is false when there is enough free space", () => {
    expect(volumeBackupsRisky({ missingBytes: String(1024 * 1024), freeBytes: String(10 * 1024 * 1024 * 1024) })).toBe(false);
  });
});

describe("volumeTempRisky", () => {
  it("is true when the largest candidate plus headroom would overflow free space", () => {
    expect(volumeTempRisky({ largestMissingBytes: String(900 * 1024 * 1024), freeBytes: String(1024 * 1024 * 1024) })).toBe(true);
  });

  it("is false when there is enough free space for the largest candidate", () => {
    expect(volumeTempRisky({ largestMissingBytes: String(1024 * 1024), freeBytes: String(10 * 1024 * 1024 * 1024) })).toBe(false);
  });
});
