import { describe, it, expect, vi, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { formatInZone, useServerClock } from "./useServerClock";

afterEach(() => vi.useRealTimers());

// The time shown is the server's, in the effective zone, and it moves on.
describe("useServerClock", () => {
  it("formats an instant in a zone, and refuses an unknown zone", () => {
    const at = new Date("2026-10-05T16:42:00Z");
    expect(formatInZone(at, "Europe/Paris", "fr")).toBe("18:42");
    expect(formatInZone(at, "America/New_York", "fr")).toBe("12:42");
    expect(formatInZone(at, "Mars/Olympus", "fr")).toBeNull();
  });

  it("advances the server's time, not the device's", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2030-01-01T00:00:00Z")); // a device clock far off
    const { result } = renderHook(() => useServerClock("2026-10-05T16:42:00Z", "Europe/Paris", "fr"));
    expect(result.current).toBe("18:42");
    act(() => vi.advanceTimersByTime(60_000));
    expect(result.current).toBe("18:43");
    const none = renderHook(() => useServerClock(undefined, "Europe/Paris", "fr"));
    expect(none.result.current).toBeNull();
  });
});
