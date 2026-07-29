import { renderHook, act } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { useSleepTimer } from "./useSleepTimer";

beforeEach(() => { vi.useFakeTimers(); });
afterEach(() => { vi.useRealTimers(); });

describe("useSleepTimer", () => {
  it("starts with no active timer", () => {
    const { result } = renderHook(() => useSleepTimer({ onMinutesFire: () => {} }));
    expect(result.current.state).toBeNull();
  });

  it("counts down minutes and fires callback at zero", () => {
    const fire = vi.fn();
    const { result } = renderHook(() => useSleepTimer({ onMinutesFire: fire }));

    act(() => { result.current.setMinutes(2); });
    expect(result.current.state).toEqual({ kind: "minutes", minutesLeft: 2 });

    act(() => { vi.advanceTimersByTime(60_000); });
    expect(result.current.state).toEqual({ kind: "minutes", minutesLeft: 1 });
    expect(fire).not.toHaveBeenCalled();

    act(() => { vi.advanceTimersByTime(60_000); });
    expect(fire).toHaveBeenCalledTimes(1);
    expect(result.current.state).toBeNull();
  });

  it("cancel clears the active timer", () => {
    const fire = vi.fn();
    const { result } = renderHook(() => useSleepTimer({ onMinutesFire: fire }));

    act(() => { result.current.setMinutes(5); });
    expect(result.current.state?.minutesLeft).toBe(5);

    act(() => { result.current.cancel(); });
    expect(result.current.state).toBeNull();

    act(() => { vi.advanceTimersByTime(10 * 60_000); });
    expect(fire).not.toHaveBeenCalled();
  });

  it("setEndOfEpisode does not start a countdown", () => {
    const fire = vi.fn();
    const { result } = renderHook(() => useSleepTimer({ onMinutesFire: fire }));

    act(() => { result.current.setEndOfEpisode(); });
    expect(result.current.state).toEqual({ kind: "endOfEpisode", minutesLeft: null });

    act(() => { vi.advanceTimersByTime(10 * 60_000); });
    expect(fire).not.toHaveBeenCalled();
    expect(result.current.state?.kind).toBe("endOfEpisode");
  });

  it("setMinutes overrides a previously set timer", () => {
    const fire = vi.fn();
    const { result } = renderHook(() => useSleepTimer({ onMinutesFire: fire }));

    act(() => { result.current.setMinutes(10); });
    act(() => { vi.advanceTimersByTime(60_000); });
    expect(result.current.state?.minutesLeft).toBe(9);

    act(() => { result.current.setMinutes(2); });
    expect(result.current.state?.minutesLeft).toBe(2);

    act(() => { vi.advanceTimersByTime(2 * 60_000); });
    expect(fire).toHaveBeenCalledTimes(1);
  });
});
