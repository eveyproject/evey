import { describe, expect, it } from "vitest";
import { videoCapacityTable } from "./videoCapacityTable";

describe("videoCapacityTable", () => {
  it("is empty without pools", () => {
    expect(videoCapacityTable(undefined)).toEqual({ columns: [], rows: [] });
  });

  it("gives one column per kind of encode and one row per output height", () => {
    const table = videoCapacityTable([
      {
        pool: "cpu",
        classes: [
          { ladderClass: "software", rungFits: [{ height: 1080, maxStreams: 2 }, { height: 720, maxStreams: 2 }] },
          { ladderClass: "software-hdr", rungFits: [{ height: 720, maxStreams: 3 }] },
        ],
      },
      { pool: "gpu", classes: [{ ladderClass: "hw", rungFits: [{ height: 2160, maxStreams: 1 }] }] },
    ]);
    expect(table.columns.map((c) => c.labelKey)).toEqual([
      "admin.transcoding.videoPoolCpu",
      "admin.systemHealth.videoCapacityCpuHdr",
      "admin.transcoding.videoPoolGpu",
    ]);
    expect(table.rows).toEqual([
      { height: 720, streams: [2, 3, undefined] },
      { height: 1080, streams: [2, undefined, undefined] },
      { height: 2160, streams: [undefined, undefined, 1] },
    ]);
  });

  it("drops a kind of encode that has no measured rung", () => {
    const table = videoCapacityTable([{ pool: "gpu", classes: [{ ladderClass: "hw", rungFits: [] }] }]);
    expect(table.columns).toEqual([]);
  });
});
