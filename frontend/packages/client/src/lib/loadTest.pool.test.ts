import { describe, it, expect } from "vitest";
import { poolShares } from "./loadTest";

describe("poolShares", () => {
  it("turns viewer counts into whole-percent shares of the audience", () => {
    expect(poolShares([{ branch: "/mnt/disk_2", viewers: 41 }, { branch: "/mnt/disk_1", viewers: 9 }])).toEqual([
      { branch: "/mnt/disk_2", viewers: 41, pct: 82 },
      { branch: "/mnt/disk_1", viewers: 9, pct: 18 },
    ]);
  });

  it("an empty audience reads as zero percent, never NaN", () => {
    expect(poolShares([{ branch: "/mnt/disk_1", viewers: 0 }])).toEqual([{ branch: "/mnt/disk_1", viewers: 0, pct: 0 }]);
    expect(poolShares([])).toEqual([]);
  });

  it("tolerates the optional fields the wire may omit", () => {
    expect(poolShares([{ viewers: 5 }, {}])).toEqual([
      { branch: "", viewers: 5, pct: 100 },
      { branch: "", viewers: 0, pct: 0 },
    ]);
  });
});
