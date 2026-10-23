import { describe, it, expect, beforeEach, vi, afterEach } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { useUpNextCountdown, COUNTDOWN_SEC } from "../hooks/useUpNextCountdown";
import {
  noteAutoAdvance,
  noteViewerActivity,
  resetStillWatchingForTests,
  setStillWatchingLimit,
  STILL_WATCHING_EXIT_MS,
  STILL_WATCHING_STEPS,
  stillWatchingLimit,
  stillWatchingPreferences,
} from "./stillWatching";

// An episode that reached its end, auto-play on: the card counts down.
function endedEpisode(nextEpisodeId: string, onPlay: (id: string) => void) {
  return renderHook(() => useUpNextCountdown({ nextEpisodeId, currentTime: 1200, duration: 1200, onPlay }));
}

function runCountdown() {
  act(() => { vi.advanceTimersByTime((COUNTDOWN_SEC + 1) * 1000); });
}

beforeEach(() => {
  vi.useFakeTimers();
  resetStillWatchingForTests();
});
afterEach(() => {
  vi.useRealTimers();
});

describe("still watching?", () => {
  it("lets three episodes follow on their own, then waits for the viewer", () => {
    const onPlay = vi.fn();
    for (const id of ["e2", "e3", "e4"]) {
      const { unmount } = endedEpisode(id, onPlay);
      runCountdown();
      unmount();
    }
    expect(onPlay.mock.calls.map((c) => c[0])).toEqual(["e2", "e3", "e4"]);

    const { result } = endedEpisode("e5", onPlay);
    runCountdown();
    expect(onPlay).toHaveBeenCalledTimes(3);
    expect(result.current.counting).toBe(false);
    expect(result.current.stillWatching).toBe(true);

    // The viewer's click on the card's button: the player notes the input,
    // the card plays, and the chain starts again.
    act(() => { noteViewerActivity(); result.current.playNow(); });
    expect(onPlay).toHaveBeenLastCalledWith("e5");
    expect(result.current.stillWatching).toBe(false);
  });

  it("starts the count again at any touch, key or click", () => {
    const onPlay = vi.fn();
    for (const id of ["e2", "e3"]) {
      const { unmount } = endedEpisode(id, onPlay);
      runCountdown();
      unmount();
    }
    noteViewerActivity();
    for (const id of ["e4", "e5", "e6"]) {
      const { unmount } = endedEpisode(id, onPlay);
      runCountdown();
      unmount();
    }
    expect(onPlay).toHaveBeenCalledTimes(5);
  });

  // The credits countdown lands on the card's playNow by itself: that is an
  // auto-advance, not the viewer, and must not reset the count.
  it("counts a move made by the countdown through playNow as automatic", () => {
    const onPlay = vi.fn();
    for (const id of ["e2", "e3", "e4"]) {
      const { result, unmount } = endedEpisode(id, onPlay);
      act(() => { noteAutoAdvance(); result.current.playNow(); });
      unmount();
    }
    const { result } = endedEpisode("e5", onPlay);
    expect(result.current.stillWatching).toBe(true);
  });

  it("never asks when the profile turned it off", () => {
    setStillWatchingLimit(0);
    const onPlay = vi.fn();
    for (const id of ["e2", "e3", "e4", "e5", "e6"]) {
      const { unmount } = endedEpisode(id, onPlay);
      runCountdown();
      unmount();
    }
    expect(onPlay).toHaveBeenCalledTimes(5);
  });

  it("asks after a single episode when the profile chose 1", () => {
    setStillWatchingLimit(1);
    const onPlay = vi.fn();
    const first = endedEpisode("e2", onPlay);
    runCountdown();
    first.unmount();
    const { result } = endedEpisode("e3", onPlay);
    runCountdown();
    expect(onPlay.mock.calls.map((c) => c[0])).toEqual(["e2"]);
    expect(result.current.stillWatching).toBe(true);
  });

  // Nobody there: the card must not keep the player and its session up until
  // the television sleeps on its own.
  it("leaves the player once the card has waited unanswered", () => {
    setStillWatchingLimit(1);
    noteAutoAdvance();
    const onTimeout = vi.fn();
    const { result } = renderHook(() =>
      useUpNextCountdown({ nextEpisodeId: "e3", currentTime: 1200, duration: 1200, onPlay: vi.fn(), onStillWatchingTimeout: onTimeout }),
    );
    expect(result.current.stillWatching).toBe(true);
    act(() => { vi.advanceTimersByTime(STILL_WATCHING_EXIT_MS - 1000); });
    expect(onTimeout).not.toHaveBeenCalled();
    act(() => { vi.advanceTimersByTime(1000); });
    expect(onTimeout).toHaveBeenCalledTimes(1);
  });

  it("does not leave when the viewer answers before the wait is over", () => {
    setStillWatchingLimit(1);
    noteAutoAdvance();
    const onTimeout = vi.fn();
    renderHook(() =>
      useUpNextCountdown({ nextEpisodeId: "e3", currentTime: 1200, duration: 1200, onPlay: vi.fn(), onStillWatchingTimeout: onTimeout }),
    );
    act(() => { vi.advanceTimersByTime(60_000); noteViewerActivity(); });
    act(() => { vi.advanceTimersByTime(STILL_WATCHING_EXIT_MS); });
    expect(onTimeout).not.toHaveBeenCalled();
  });

  it("offers asking after every episode", () => {
    expect(STILL_WATCHING_STEPS).toEqual([0, 1, 2, 3, 5]);
  });

  it("reads and writes the threshold in the profile preferences", () => {
    expect(stillWatchingLimit(undefined)).toBe(3);
    expect(stillWatchingLimit({ still_watching_after: "0" })).toBe(0);
    expect(stillWatchingLimit({ still_watching_after: "banana" })).toBe(3);
    expect(stillWatchingPreferences({ audio_languages: "fr" }, 5)).toEqual({ audio_languages: "fr", still_watching_after: "5" });
  });
});
