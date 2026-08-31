import { describe, it, expect } from "vitest";
import {
  audioDelayed,
  baseName,
  isAckable,
  repairableCount,
  triageOf,
  verdictOf,
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

describe("verdictOf", () => {
  it("trusts the server's verdict when present", () => {
    expect(verdictOf({ verdict: "manual", hasIndex: false })).toBe("manual");
  });

  // Same table as the Go SeekStatus.Verdict: what only the operator can fix
  // wins over what Evey could fix, which wins over a mere fact.
  it("derives the Go precedence from the flags for an older server", () => {
    expect(verdictOf({ hasIndex: true })).toBe("healthy");
    expect(verdictOf({ hasIndex: false })).toBe("repairable");
    expect(verdictOf({ hasIndex: true, damagedBytes: "512" })).toBe("repairable");
    expect(verdictOf({ hasIndex: true, audioDelaySec: 0.9 })).toBe("repairable");
    expect(verdictOf({ hasIndex: true, wrongContainer: true })).toBe("repairable");
    expect(verdictOf({ hasIndex: false, truncated: true })).toBe("manual");
    expect(verdictOf({ hasIndex: false, seekCoarse: true })).toBe("manual");
    expect(verdictOf({ hasIndex: true, audioShortfallSec: 40 })).toBe("informational");
    expect(verdictOf({ hasIndex: true, trailingJunk: true })).toBe("informational");
    expect(verdictOf({ hasIndex: true, trailingJunk: true, audioDelaySec: 0.5 })).toBe("repairable");
  });

  it("only lets what nobody can repair be acknowledged", () => {
    expect(isAckable({ hasIndex: false })).toBe(false);
    expect(isAckable({ hasIndex: true, trailingJunk: true })).toBe(true);
    expect(isAckable({ truncated: true })).toBe(true);
  });
});

describe("triageOf", () => {
  it("counts like the server rollups: acknowledged files leave their pile", () => {
    const t = triageOf([
      { hasIndex: false },
      { hasIndex: true, audioDelaySec: 1 },
      { hasIndex: true, truncated: true },
      { hasIndex: true, truncated: true, ackedAtMs: "1700000000000" },
      { hasIndex: true, trailingJunk: true },
      { hasIndex: true },
    ]);
    expect(t).toEqual({ repairable: 2, manual: 1, informational: 1, acked: 1 });
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
