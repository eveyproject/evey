import { describe, it, expect } from "vitest";
import { scanFailedPaths } from "./scanReport";

describe("scanFailedPaths", () => {
  it("reads the roots the scan could not read", () => {
    expect(scanFailedPaths({ result: { phase: "SCAN_PHASE_COMPLETE", failedPaths: ["/music/a"] } })).toEqual(["/music/a"]);
  });

  it("is empty for a complete scan or an unexpected body", () => {
    expect(scanFailedPaths({ result: { phase: "SCAN_PHASE_COMPLETE" } })).toEqual([]);
    expect(scanFailedPaths(undefined)).toEqual([]);
    expect(scanFailedPaths({ result: { failedPaths: "nope" } })).toEqual([]);
  });
});
