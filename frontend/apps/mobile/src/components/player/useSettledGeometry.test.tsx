import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import {
  useSettledGeometry,
  GEOMETRY_SETTLE_MS,
  GEOMETRY_MAX_HOLD_MS,
  type PlayerGeometry,
} from "./useSettledGeometry";

const PORTRAIT: PlayerGeometry = { width: 412, height: 915, insets: { top: 24, right: 0, bottom: 48, left: 0 } };
const LANDSCAPE: PlayerGeometry = { width: 915, height: 412, insets: { top: 0, right: 36, bottom: 24, left: 48 } };

beforeEach(() => { vi.useFakeTimers(); });
afterEach(() => { vi.useRealTimers(); });

describe("useSettledGeometry", () => {
  it("publishes the mount geometry immediately", () => {
    const { result } = renderHook(() => useSettledGeometry(PORTRAIT));
    expect(result.current.geometry).toEqual(PORTRAIT);
    expect(result.current.resizing).toBe(false);
  });

  it("coalesces a size change and a late inset change into ONE publish", () => {
    const { result, rerender } = renderHook((g: PlayerGeometry) => useSettledGeometry(g), {
      initialProps: PORTRAIT,
    });

    // The window rotates first, the insets follow two frames later.
    rerender({ ...LANDSCAPE, insets: PORTRAIT.insets });
    expect(result.current.resizing).toBe(true);
    expect(result.current.geometry).toEqual(PORTRAIT); // nothing published yet

    act(() => { vi.advanceTimersByTime(GEOMETRY_SETTLE_MS - 20); });
    rerender(LANDSCAPE);
    // The second event re-arms the window: no intermediate publish.
    act(() => { vi.advanceTimersByTime(GEOMETRY_SETTLE_MS - 1); });
    expect(result.current.geometry).toEqual(PORTRAIT);

    act(() => { vi.advanceTimersByTime(1); });
    expect(result.current.geometry).toEqual(LANDSCAPE);
    expect(result.current.resizing).toBe(false);
  });

  it("never holds longer than the ceiling, even while events keep coming", () => {
    const { result, rerender } = renderHook((g: PlayerGeometry) => useSettledGeometry(g), {
      initialProps: PORTRAIT,
    });

    let bottom = 0;
    for (let elapsed = 0; elapsed < GEOMETRY_MAX_HOLD_MS + GEOMETRY_SETTLE_MS; elapsed += 50) {
      rerender({ ...LANDSCAPE, insets: { ...LANDSCAPE.insets, bottom: bottom++ } });
      act(() => { vi.advanceTimersByTime(50); });
    }
    expect(result.current.resizing).toBe(false);
  });

  it("does not flag a resize when only the insets move", () => {
    const { result, rerender } = renderHook((g: PlayerGeometry) => useSettledGeometry(g), {
      initialProps: PORTRAIT,
    });

    // System bars appearing with the controls: same window, new insets. The bar
    // must stay painted where it is, then move once - never blink.
    rerender({ ...PORTRAIT, insets: { ...PORTRAIT.insets, bottom: 0 } });
    expect(result.current.resizing).toBe(false);
    expect(result.current.geometry).toEqual(PORTRAIT);

    act(() => { vi.advanceTimersByTime(GEOMETRY_SETTLE_MS); });
    expect(result.current.geometry.insets.bottom).toBe(0);
  });
});
