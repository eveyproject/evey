import { describe, expect, it } from "vitest";
import { activeSkippable, skippableCandidates } from "./skipSegments";

describe("activeSkippable", () => {
  it("takes the earliest segment when two claim the same seconds", () => {
    const got = activeSkippable(
      [
        { type: "recap", startSec: 40, endSec: 80 },
        { type: "intro", startSec: 30, endSec: 90 },
      ],
      50,
    );
    expect(got?.type).toBe("intro");
  });

  it("ignores a segment too short to be worth a button, and an empty one", () => {
    expect(activeSkippable([{ type: "intro", startSec: 10, endSec: 13 }], 11)).toBeNull();
    expect(activeSkippable([{ type: "intro", startSec: 10, endSec: 10 }], 10)).toBeNull();
  });

  it("keeps only the skippable types and converts to seconds", () => {
    expect(skippableCandidates([
      { type: "intro", startMs: 1000, endMs: 20000 },
      { type: "chapter", startMs: 0, endMs: 5000 },
    ])).toEqual([{ type: "intro", startSec: 1, endSec: 20 }]);
  });
});

describe("skippableFromChapters", () => {
  it("recognises the chapters the server's detection would name, and ignores the others", async () => {
    const { skippableFromChapters } = await import("./skipSegments");
    const out = skippableFromChapters([
      { title: "Opening", startMs: "0", endMs: "90000" },
      { title: "Chapter 2", startMs: "90000", endMs: "600000" },
      { title: "Previously on", startMs: "600000", endMs: "640000" },
      { title: "Ending", startMs: "2500000", endMs: "2590000" },
      { title: "  ", startMs: "1", endMs: "2" },
    ]);
    expect(out.map((c) => c.type)).toEqual(["intro", "recap", "end_credits"]);
    expect(out[0]).toEqual({ type: "intro", startSec: 0, endSec: 90 });
  });
});
