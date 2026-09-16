import { describe, it, expect } from "vitest";
import { nudgeOnResume, shouldNudgeOnResume } from "./resumeNudge";

describe("resume nudge", () => {
  it("only after a pause long enough for the decoder to have been parked, on a seekable element", () => {
    expect(shouldNudgeOnResume(500, false, 4)).toBe(false);
    expect(shouldNudgeOnResume(4000, false, 4)).toBe(true);
    expect(shouldNudgeOnResume(60_000, true, 4)).toBe(false);
    expect(shouldNudgeOnResume(60_000, false, 1)).toBe(false);
  });

  it("seeks in place on a qualifying resume and leaves a quick pause alone", () => {
    const video = { currentTime: 100, seeking: false, readyState: 4 } as unknown as HTMLVideoElement;
    expect(nudgeOnResume(video, 10_000, 15_000)).toBe(true);
    expect(video.currentTime).toBeCloseTo(100.01);
    expect(nudgeOnResume(video, 14_800, 15_000)).toBe(false);
    expect(nudgeOnResume(video, null, 15_000)).toBe(false);
    expect(video.currentTime).toBeCloseTo(100.01);
  });
});
