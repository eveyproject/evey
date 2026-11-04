import { describe, expect, it } from "vitest";
import { QueryClient } from "@tanstack/react-query";
import { isProgressViewQuery, refreshProgressViews } from "./progressRefresh";

describe("refreshProgressViews", () => {
  it("targets a title's progress and the resume row only", () => {
    expect(isProgressViewQuery(["/api/v1/playback/m1/progress"])).toBe(true);
    expect(isProgressViewQuery(["/api/v1/playback/resume", { limit: 20 }])).toBe(true);
    expect(isProgressViewQuery(["/api/v1/playback/m1/stream-info"])).toBe(false);
    expect(isProgressViewQuery(["/api/v1/media/m1"])).toBe(false);
  });

  it("marks them stale so the page on screen reads them again", () => {
    const qc = new QueryClient();
    qc.setQueryData(["/api/v1/playback/m1/progress"], { positionMs: "0" });
    qc.setQueryData(["/api/v1/media/m1"], { id: "m1" });
    refreshProgressViews(qc);
    expect(qc.getQueryState(["/api/v1/playback/m1/progress"])?.isInvalidated).toBe(true);
    expect(qc.getQueryState(["/api/v1/media/m1"])?.isInvalidated).toBe(false);
    expect(() => refreshProgressViews(undefined)).not.toThrow();
  });
});
