import { describe, it, expect, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { useCreditsCountdown } from "./useCreditsCountdown";
import type { ResolvedSegment } from "../lib/skipSegments";

const credits: ResolvedSegment = { type: "end_credits", startSec: 10, endSec: 60 };

type Props = { currentTime: number; segment: ResolvedSegment | null; mediaId: string };

function mount(currentTime: number, overrides: Partial<Parameters<typeof useCreditsCountdown>[0]> = {}) {
  const onNextEpisode = vi.fn();
  const hook = renderHook(
    (props: Props) => useCreditsCountdown({ enabled: true, onNextEpisode, ...overrides, ...props }),
    { initialProps: { currentTime, segment: credits, mediaId: "m1" } },
  );
  // Playback as the players report it: a tick every half second. A single
  // rerender far ahead would read as a seek, which is its own test below.
  const playTo = (to: number, from: number, segment: ResolvedSegment | null = credits, mediaId = "m1") => {
    for (let t = from + 0.5; t < to; t += 0.5) hook.rerender({ currentTime: t, segment, mediaId });
    hook.rerender({ currentTime: to, segment, mediaId });
  };
  return { ...hook, onNextEpisode, playTo };
}

describe("useCreditsCountdown", () => {
  it("counts down on the playhead from the start of the credits when the viewer plays into them", () => {
    const { result, playTo } = mount(10);
    expect(result.current.counting).toBe(true);
    expect(result.current.remaining).toBe(15);
    playTo(15.2, 10);
    expect(result.current.remaining).toBe(10);
  });

  it("gives a viewer who resumes inside the credits the full countdown, capped by the segment end", () => {
    const { result, playTo, onNextEpisode } = mount(40);
    expect(result.current.remaining).toBe(15);
    playTo(50, 40);
    expect(result.current.remaining).toBe(5);
    expect(onNextEpisode).not.toHaveBeenCalled();

    // Entered 10s before the end: the clock cannot outlive the credits.
    const late = mount(52);
    expect(late.result.current.remaining).toBe(8);
  });

  // The page reloads at zero and lands inside the credits in one jump: that
  // is the resume, not a seek the viewer made inside them.
  it("treats the jump from a fresh load into the credits as a resume, and counts", () => {
    const { result, rerender } = renderHook(
      (props: Props) => useCreditsCountdown({ enabled: true, onNextEpisode: vi.fn(), ...props }),
      { initialProps: { currentTime: 0, segment: null, mediaId: "m1" } },
    );
    rerender({ currentTime: 44, segment: credits, mediaId: "m1" });
    expect(result.current.counting).toBe(true);
    expect(result.current.remaining).toBe(15);
  });

  it("fires once when the countdown reaches zero and not again on later ticks", () => {
    const { playTo, onNextEpisode } = mount(10);
    playTo(26, 10);
    expect(onNextEpisode).toHaveBeenCalledTimes(1);
  });

  it("Watch credits cancels the countdown for these credits, a seek back into them included", () => {
    const { result, rerender, playTo, onNextEpisode } = mount(10);
    act(() => result.current.watchCredits());
    expect(result.current.counting).toBe(false);
    expect(result.current.remaining).toBeNull();

    rerender({ currentTime: 5, segment: null, mediaId: "m1" });
    rerender({ currentTime: 30, segment: credits, mediaId: "m1" });
    expect(result.current.counting).toBe(false);
    playTo(59, 30);
    expect(onNextEpisode).not.toHaveBeenCalled();
  });

  // Prod 2026-10-15: the viewer scrubbed inside credits marked four minutes
  // early and nothing they did read as "I am still watching".
  it("a seek inside the credits while the clock runs cancels it, forward or back", () => {
    const fwd = mount(10);
    fwd.playTo(12, 10);
    fwd.rerender({ currentTime: 30, segment: credits, mediaId: "m1" });
    expect(fwd.result.current.counting).toBe(false);
    fwd.playTo(59, 30);
    expect(fwd.onNextEpisode).not.toHaveBeenCalled();

    const back = mount(40);
    back.playTo(42, 40);
    back.rerender({ currentTime: 20, segment: credits, mediaId: "m1" });
    expect(back.result.current.counting).toBe(false);
    expect(back.result.current.remaining).toBeNull();
  });

  it("a seek that lands past the clock cancels instead of firing", () => {
    const { playTo, rerender, onNextEpisode } = mount(10);
    playTo(22, 10);
    rerender({ currentTime: 50, segment: credits, mediaId: "m1" });
    rerender({ currentTime: 50.5, segment: credits, mediaId: "m1" });
    expect(onNextEpisode).not.toHaveBeenCalled();
  });

  it("restarts the clock when the viewer leaves the credits and plays back into them", () => {
    const { result, rerender, playTo } = mount(10);
    playTo(20, 10);
    expect(result.current.remaining).toBe(5);
    rerender({ currentTime: 5, segment: null, mediaId: "m1" });
    rerender({ currentTime: 10, segment: credits, mediaId: "m1" });
    expect(result.current.remaining).toBe(15);
  });

  it("never counts with auto-play off, without a next episode, or outside the credits", () => {
    const off = mount(10, { enabled: false });
    expect(off.result.current.counting).toBe(false);
    expect(off.result.current.remaining).toBeNull();
    off.playTo(59, 10);
    expect(off.onNextEpisode).not.toHaveBeenCalled();

    const noNext = mount(10, { onNextEpisode: undefined });
    expect(noNext.result.current.counting).toBe(false);

    const intro = mount(10, {});
    intro.rerender({ currentTime: 10, segment: { type: "intro", startSec: 0, endSec: 60 }, mediaId: "m1" });
    expect(intro.result.current.counting).toBe(false);
  });

  it("a new episode is a new cycle: the cancel is forgotten and the countdown may fire again", () => {
    const { result, rerender, playTo, onNextEpisode } = mount(10);
    act(() => result.current.watchCredits());
    rerender({ currentTime: 10, segment: credits, mediaId: "m2" });
    expect(result.current.counting).toBe(true);
    playTo(25, 10, credits, "m2");
    expect(onNextEpisode).toHaveBeenCalledTimes(1);
  });
});
