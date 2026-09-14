import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fadeClasses, useFadeParked } from "./useFadeParked";

describe("useFadeParked", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("parks only after the fade has run, and un-parks at once on reveal", () => {
    const { result, rerender } = renderHook(({ v }) => useFadeParked(v, 300), { initialProps: { v: true } });
    expect(result.current).toBe(false);

    rerender({ v: false });
    // Fading: still displayed so the opacity transition has a box.
    expect(result.current).toBe(false);
    act(() => { vi.advanceTimersByTime(299); });
    expect(result.current).toBe(false);
    act(() => { vi.advanceTimersByTime(1); });
    expect(result.current).toBe(true);

    rerender({ v: true });
    expect(result.current).toBe(false);
  });

  it("a reveal during the fade cancels the park", () => {
    const { result, rerender } = renderHook(({ v }) => useFadeParked(v, 300), { initialProps: { v: true } });
    rerender({ v: false });
    act(() => { vi.advanceTimersByTime(150); });
    rerender({ v: true });
    act(() => { vi.advanceTimersByTime(500); });
    expect(result.current).toBe(false);
  });

  it("fadeClasses spells the three states", () => {
    expect(fadeClasses(true, false)).toBe("fade-shown");
    expect(fadeClasses(false, false)).toBe("fade-hidden");
    expect(fadeClasses(false, true)).toBe("fade-hidden fade-parked");
  });
});
