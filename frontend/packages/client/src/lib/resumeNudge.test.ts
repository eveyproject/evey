import { describe, it, expect, vi } from "vitest";
import { resumeNudger, shouldNudgeOnResume } from "./resumeNudge";

describe("resume nudge", () => {
  it("qualifies only after a pause long enough for the codec to have been reset", () => {
    expect(shouldNudgeOnResume(500)).toBe(false);
    expect(shouldNudgeOnResume(4000)).toBe(true);
  });

  it("seeks in place on a long pause's resume and leaves a quick one alone", () => {
    let t = 1000;
    const seekBy = vi.fn();
    const onPlaying = resumeNudger(seekBy, () => t);
    onPlaying(true); // initial start: never a resume
    expect(seekBy).not.toHaveBeenCalled();
    onPlaying(false); t += 1500; onPlaying(true);
    expect(seekBy).not.toHaveBeenCalled();
    onPlaying(false); onPlaying(false); t += 300_000; onPlaying(true);
    expect(seekBy).toHaveBeenCalledTimes(1);
    expect(seekBy).toHaveBeenCalledWith(0.01);
    onPlaying(true); // repeated playing=true without a pause: nothing
    expect(seekBy).toHaveBeenCalledTimes(1);
  });
});
