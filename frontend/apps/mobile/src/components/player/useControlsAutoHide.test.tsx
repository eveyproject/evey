import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useControlsAutoHide, CONTROLS_HIDE_DELAY_MS } from "./useControlsAutoHide";

beforeEach(() => { vi.useFakeTimers(); });
afterEach(() => { vi.useRealTimers(); });

describe("useControlsAutoHide", () => {
  it("uses a 4500ms delay", () => {
    expect(CONTROLS_HIDE_DELAY_MS).toBe(4500);
  });

  it("hides after the delay while playing with no panel open", () => {
    const { result } = renderHook(() => useControlsAutoHide({ isPlaying: true, hasOpenPanel: false }));
    expect(result.current.showControls).toBe(true);

    act(() => { vi.advanceTimersByTime(CONTROLS_HIDE_DELAY_MS - 1); });
    expect(result.current.showControls).toBe(true);

    act(() => { vi.advanceTimersByTime(1); });
    expect(result.current.showControls).toBe(false);
  });

  it("resetHideTimer re-arms a full delay instead of letting a stale timer fire early", () => {
    const { result } = renderHook(() => useControlsAutoHide({ isPlaying: true, hasOpenPanel: false }));

    act(() => { vi.advanceTimersByTime(3000); });
    act(() => { result.current.resetHideTimer(); }); // simulates a tap/seek interaction
    expect(result.current.showControls).toBe(true);

    act(() => { vi.advanceTimersByTime(3000); });
    expect(result.current.showControls).toBe(true); // would have hidden without the reset

    act(() => { vi.advanceTimersByTime(CONTROLS_HIDE_DELAY_MS - 3000); });
    expect(result.current.showControls).toBe(false);
  });

  it("never hides while a panel/sheet is open, even far past the delay", () => {
    const { result } = renderHook(() => useControlsAutoHide({ isPlaying: true, hasOpenPanel: true }));

    act(() => { vi.advanceTimersByTime(CONTROLS_HIDE_DELAY_MS * 5); });
    expect(result.current.showControls).toBe(true);
  });

  it("never hides while paused, even far past the delay", () => {
    const { result } = renderHook(() => useControlsAutoHide({ isPlaying: false, hasOpenPanel: false }));

    act(() => { vi.advanceTimersByTime(CONTROLS_HIDE_DELAY_MS * 5); });
    expect(result.current.showControls).toBe(true);
  });

  it("re-arms the hide timer once a panel closes while still playing", () => {
    const { result, rerender } = renderHook(
      ({ hasOpenPanel }) => useControlsAutoHide({ isPlaying: true, hasOpenPanel }),
      { initialProps: { hasOpenPanel: true } },
    );

    act(() => { vi.advanceTimersByTime(CONTROLS_HIDE_DELAY_MS * 2); });
    expect(result.current.showControls).toBe(true);

    rerender({ hasOpenPanel: false });
    act(() => { vi.advanceTimersByTime(CONTROLS_HIDE_DELAY_MS - 1); });
    expect(result.current.showControls).toBe(true);
    act(() => { vi.advanceTimersByTime(1); });
    expect(result.current.showControls).toBe(false);
  });

  it("re-arms the hide timer once playback resumes", () => {
    const { result, rerender } = renderHook(
      ({ isPlaying }) => useControlsAutoHide({ isPlaying, hasOpenPanel: false }),
      { initialProps: { isPlaying: false } },
    );

    act(() => { vi.advanceTimersByTime(CONTROLS_HIDE_DELAY_MS * 2); });
    expect(result.current.showControls).toBe(true);

    rerender({ isPlaying: true });
    act(() => { vi.advanceTimersByTime(CONTROLS_HIDE_DELAY_MS - 1); });
    expect(result.current.showControls).toBe(true);
    act(() => { vi.advanceTimersByTime(1); });
    expect(result.current.showControls).toBe(false);
  });

  it("toggleControls hides immediately then re-shows and re-arms", () => {
    const { result } = renderHook(() => useControlsAutoHide({ isPlaying: true, hasOpenPanel: false }));

    act(() => { result.current.toggleControls(); });
    expect(result.current.showControls).toBe(false);

    act(() => { result.current.toggleControls(); });
    expect(result.current.showControls).toBe(true);

    act(() => { vi.advanceTimersByTime(CONTROLS_HIDE_DELAY_MS); });
    expect(result.current.showControls).toBe(false);
  });
});
