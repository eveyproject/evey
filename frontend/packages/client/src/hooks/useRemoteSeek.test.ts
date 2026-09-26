import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { HOLD_SCRUB_TICK_MS, SEEK_COMMIT_DELAY_MS } from "../lib/seekGesture";
import { useRemoteSeek } from "./useRemoteSeek";

let position = 100;
const seekTo = vi.fn((sec: number) => { position = sec; });

function setup() {
  return renderHook(({ pos }) => useRemoteSeek({
    positionSec: pos,
    getPosition: () => position,
    getDuration: () => 3600,
    seekTo,
  }), { initialProps: { pos: position } });
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.clearAllMocks();
  position = 100;
});
afterEach(() => vi.useRealTimers());

describe("useRemoteSeek", () => {
  it("adds taps up into a preview and lands ONE seek once the pressing stops", () => {
    const { result } = setup();
    act(() => { result.current.bump(30); result.current.bump(30); result.current.bump(30); });
    // 30 + 30 + 60: the third tap takes the doubled step.
    expect(result.current.previewSec).toBe(220);
    expect(seekTo).not.toHaveBeenCalled();
    act(() => { vi.advanceTimersByTime(SEEK_COMMIT_DELAY_MS); });
    expect(seekTo).toHaveBeenCalledTimes(1);
    expect(seekTo).toHaveBeenCalledWith(220);
  });

  it("keeps the preview until the real playhead has caught up", () => {
    const { result, rerender } = setup();
    act(() => { result.current.bump(-30); vi.advanceTimersByTime(SEEK_COMMIT_DELAY_MS); });
    expect(result.current.previewSec).toBe(70);
    rerender({ pos: 100 });
    expect(result.current.previewSec).toBe(70);
    rerender({ pos: 70.5 });
    expect(result.current.previewSec).toBeNull();
  });

  it("scrubs while a key is held and commits on release", () => {
    const { result } = setup();
    act(() => { result.current.beginHold(1); vi.advanceTimersByTime(10 * HOLD_SCRUB_TICK_MS); });
    expect(result.current.isHolding()).toBe(true);
    expect(result.current.previewSec).toBeGreaterThan(150);
    act(() => { result.current.endHold(30); });
    expect(seekTo).not.toHaveBeenCalled();
    act(() => { vi.advanceTimersByTime(SEEK_COMMIT_DELAY_MS); });
    expect(seekTo).toHaveBeenCalledTimes(1);
  });

  it("reads a hold released before its first tick as a tap", () => {
    const { result } = setup();
    act(() => { result.current.beginHold(-1); result.current.endHold(10); });
    expect(result.current.previewSec).toBe(90);
  });

  it("lands the pending seek on demand, and says when there was none", () => {
    const { result } = setup();
    let landed = false;
    act(() => { landed = result.current.commitNow(); });
    expect(landed).toBe(false);
    act(() => { result.current.bump(30); landed = result.current.commitNow(); });
    expect(landed).toBe(true);
    expect(seekTo).toHaveBeenCalledWith(130);
    act(() => { vi.advanceTimersByTime(SEEK_COMMIT_DELAY_MS); });
    expect(seekTo).toHaveBeenCalledTimes(1);
  });

  it("never previews outside the timeline", () => {
    const { result } = setup();
    act(() => { result.current.bump(-300); });
    expect(result.current.previewSec).toBe(0);
  });
});
