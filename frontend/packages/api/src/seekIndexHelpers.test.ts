import { describe, it, expect } from "vitest";
import {
  audioDelayed,
  baseName,
  brokenInterleave,
  isAckable,
  matchesView,
  needsResync,
  repairableCount,
  sortSeekItems,
  triageOf,
  triageRows,
  verdictOf,
  viewCounts,
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

describe("brokenInterleave", () => {
  it("only flags a cluster timestamp stepping back further than the floor", () => {
    expect(brokenInterleave({})).toBe(false);
    expect(brokenInterleave({ interleaveBackstepSec: 4 })).toBe(false);
    expect(brokenInterleave({ interleaveBackstepSec: 366 })).toBe(true);
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
    expect(verdictOf({ verdict: "informational", hasIndex: false })).toBe("informational");
    // A server still speaking the retired "your call" verdict is read as the
    // remark it always was - never as a repair.
    expect(verdictOf({ verdict: "manual", hasIndex: false })).toBe("informational");
  });

  // Same table as the Go SeekStatus.Verdict: what Evey repairs here, and
  // everything else - which is a remark, never an instruction.
  it("derives the Go precedence from the flags for an older server", () => {
    expect(verdictOf({ hasIndex: true })).toBe("healthy");
    expect(verdictOf({ hasIndex: false })).toBe("repairable");
    expect(verdictOf({ hasIndex: true, damagedBytes: "512" })).toBe("repairable");
    // An offset alone is a fact about the file, never a repair: rewriting one
    // erased a deliberate 12s dub alignment in prod (a prod title).
    expect(verdictOf({ hasIndex: true, audioDelaySec: 0.9 })).toBe("informational");
    // Tracks measured to disagree are worth saying, and still nothing more.
    expect(
      verdictOf({ hasIndex: true, audioDelaySec: 12, audioTracksLagSec: 11.9, audioTracksChecked: true }),
    ).toBe("informational");
    // Proven coherent: the offset is deliberate, so it stays a plain fact.
    expect(
      verdictOf({ hasIndex: true, audioDelaySec: 12, audioTracksLagSec: 0, audioTracksChecked: true }),
    ).toBe("informational");
    expect(verdictOf({ hasIndex: true, wrongContainer: true })).toBe("repairable");
    expect(verdictOf({ hasIndex: false, truncated: true })).toBe("informational");
    expect(verdictOf({ hasIndex: false, seekCoarse: true })).toBe("informational");
    expect(verdictOf({ hasIndex: true, audioShortfallSec: 40 })).toBe("informational");
    expect(verdictOf({ hasIndex: true, trailingJunk: true })).toBe("informational");
    expect(verdictOf({ hasIndex: true, trailingJunk: true, audioDelaySec: 0.5 })).toBe("informational");
    expect(verdictOf({ hasIndex: true, interleaveBackstepSec: 366 })).toBe("repairable");
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
    // The audio-offset file counts as information now, not as a repair, and
    // so does the truncated one: there is no third pile any more.
    expect(t).toEqual({ repairable: 1, informational: 3, acked: 1 });
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

describe("views, search and sort of the displayed rows", () => {
  const rows = [
    { mediaId: "a", title: "Alpha", path: "/m/alpha.mkv", hasIndex: false, sizeBytes: "100" }, // repairable
    { mediaId: "b", title: "Bravo", path: "/m/bravo.mkv", hasIndex: true, truncated: true, sizeBytes: "900" }, // informational
    { mediaId: "c", title: "Charlie", path: "/m/charlie.mkv", hasIndex: true, trailingJunk: true, sizeBytes: "300", ackedAtMs: "1" }, // info, acked
    { mediaId: "d", title: "Delta", path: "/m/delta.mkv", hasIndex: true, hasBackup: true, sizeBytes: "50" }, // healthy + backup
    { mediaId: "e", title: "Echo", path: "/m/echo.mkv", hasIndex: true, sizeBytes: "10" }, // healthy
    { mediaId: "f", title: "Foxtrot", path: "/m/foxtrot.mkv", hasIndex: true, audioShortfallSec: 40, sizeBytes: "700" }, // info
  ];

  it("attention = pending verdicts plus backups, never healthy or acknowledged", () => {
    expect(rows.filter((r) => matchesView(r, "attention")).map((r) => r.mediaId)).toEqual(["a", "b", "d", "f"]);
  });

  it("counts every view from the same rows", () => {
    expect(viewCounts(rows)).toEqual({ attention: 4, repairable: 1, informational: 3, acked: 1, backups: 1, all: 6 });
  });

  it("priority sort: repairable, information (acknowledged last), healthy - biggest first inside a rank", () => {
    expect(sortSeekItems(rows, "priority").map((r) => r.mediaId)).toEqual(["a", "b", "f", "c", "d", "e"]);
  });

  it("size and title sorts", () => {
    expect(sortSeekItems(rows, "size").map((r) => r.mediaId)).toEqual(["b", "f", "c", "a", "d", "e"]);
    expect(sortSeekItems(rows, "title").map((r) => r.title)).toEqual(["Alpha", "Bravo", "Charlie", "Delta", "Echo", "Foxtrot"]);
  });

  it("search matches title or path, case-insensitively, and composes with the view", () => {
    expect(triageRows(rows, "all", "ECHO", "priority").map((r) => r.mediaId)).toEqual(["e"]);
    expect(triageRows(rows, "all", "/m/", "title")).toHaveLength(6);
    expect(triageRows(rows, "informational", "fox", "priority").map((r) => r.mediaId)).toEqual(["f"]);
  });
});

// Mirrors the Go maintenance.needsResyncForDamage: the button a row offers and
// the file a batch acts on must never disagree, or the operator clicks a repair
// the server will not run (or the batch quietly skips a file the row promised).
describe("needsResync", () => {
  it("takes the scan's measured mid-file corruption", () => {
    expect(needsResync({ hasIndex: true, damagedBytes: "4096" })).toBe(true);
    expect(needsResync({ hasIndex: true, damagedBytes: "0" })).toBe(false);
  });

  // The case the 2026-09-07 prod batch exposed: a repair walked the file, the
  // source refused to parse, and nothing carried that answer forward.
  it("takes a file the strict repair already refused, whatever its index says", () => {
    expect(needsResync({ resyncRequired: true, hasIndex: false })).toBe(true);
    expect(needsResync({ resyncRequired: true, hasIndex: true })).toBe(true);
  });

  it("never offers a resync where there is nothing to resynchronise onto", () => {
    expect(needsResync({ resyncRequired: true, truncated: true })).toBe(false);
    expect(needsResync({ resyncRequired: true, seekCoarse: true })).toBe(false);
  });
});

describe("verdictOf with a repair-discovered corruption", () => {
  it("is repairable - a resync is a repair Evey runs here", () => {
    expect(verdictOf({ hasIndex: true, resyncRequired: true })).toBe("repairable");
  });
});
