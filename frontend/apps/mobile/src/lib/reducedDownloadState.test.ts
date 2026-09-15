import { describe, it, expect } from "vitest";
import {
  cancelReducedDownload,
  claimReducedJob,
  getReducedPreparePermille,
  getReducedSliceProgress,
  isReducedDownloadRunning,
  releaseReducedJob,
} from "./reducedDownloadState";

// One job per item, readable by the screens without importing the download
// flow, and a cancel that wakes whatever is waiting.
describe("reducedDownloadState", () => {
  it("claims an item once and releases it", () => {
    const job = claimReducedJob("m1", 12);
    expect(job).not.toBeNull();
    expect(claimReducedJob("m1", 12)).toBeNull();
    expect(isReducedDownloadRunning("m1")).toBe(true);
    expect(isReducedDownloadRunning("m2")).toBe(false);

    releaseReducedJob("m1");
    expect(isReducedDownloadRunning("m1")).toBe(false);
    expect(claimReducedJob("m1", 3)).not.toBeNull();
    releaseReducedJob("m1");
  });

  it("exposes the live counters, and zeros for an item with no job", () => {
    const job = claimReducedJob("m3", 40)!;
    expect(getReducedPreparePermille("m3")).toBe(0);
    expect(getReducedSliceProgress("m3")).toEqual({ collected: 0, slices: 40 });

    job.preparePermille = 375;
    job.collected = 7;
    expect(getReducedPreparePermille("m3")).toBe(375);
    expect(getReducedSliceProgress("m3")).toEqual({ collected: 7, slices: 40 });

    releaseReducedJob("m3");
    expect(getReducedPreparePermille("m3")).toBe(0);
    expect(getReducedSliceProgress("m3")).toEqual({ collected: 0, slices: 0 });
  });

  it("cancel flags the job and wakes the poll and the transfer", () => {
    expect(cancelReducedDownload("nobody")).toBe(false);

    const job = claimReducedJob("m4", 5)!;
    let woken = 0;
    let stopped = 0;
    job.wake = () => { woken++; };
    job.stopTransfer = () => { stopped++; };

    expect(cancelReducedDownload("m4")).toBe(true);
    expect(job.cancelled).toBe(true);
    expect(woken).toBe(1);
    expect(stopped).toBe(1);
    // Still claimed until the flow releases it: no second collection may start.
    expect(isReducedDownloadRunning("m4")).toBe(true);
    releaseReducedJob("m4");
  });
});
