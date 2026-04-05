import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook } from "@testing-library/react";

const reportProgressMock = vi.fn();
vi.mock("@evey/api", () => ({
  playbackServiceReportProgress: (...args: unknown[]) => reportProgressMock(...args),
}));

import { useProgressReporter } from "./useProgressReporter";

beforeEach(() => {
  vi.useFakeTimers();
  reportProgressMock.mockReset();
  reportProgressMock.mockResolvedValue(undefined);
});
afterEach(() => { vi.useRealTimers(); });

describe("useProgressReporter", () => {
  it("does not report when sessionId is null", () => {
    renderHook(() => useProgressReporter(null, () => 0));
    vi.advanceTimersByTime(20_000);
    expect(reportProgressMock).not.toHaveBeenCalled();
  });

  it("reports position every 10s", () => {
    let pos = 5000;
    renderHook(() => useProgressReporter("s1", () => pos));

    vi.advanceTimersByTime(10_000);
    expect(reportProgressMock).toHaveBeenCalledTimes(1);
    expect(reportProgressMock).toHaveBeenCalledWith({
      sessionId: "s1",
      positionMs: "5000",
      isPaused: false,
      isMuted: false,
      volumeLevel: 100,
    });

    pos = 15000;
    vi.advanceTimersByTime(10_000);
    expect(reportProgressMock).toHaveBeenCalledTimes(2);
    expect(reportProgressMock).toHaveBeenLastCalledWith(expect.objectContaining({
      positionMs: "15000",
    }));
  });

  it("skips report when paused", () => {
    renderHook(() => useProgressReporter("s1", () => 1000, () => true));

    vi.advanceTimersByTime(10_000);
    expect(reportProgressMock).not.toHaveBeenCalled();
  });

  it("includes volume and muted state", () => {
    renderHook(() =>
      useProgressReporter("s1", () => 3000, () => false, () => 0.5, () => true),
    );

    vi.advanceTimersByTime(10_000);
    expect(reportProgressMock).toHaveBeenCalledWith(expect.objectContaining({
      volumeLevel: 50,
      isMuted: true,
    }));
  });

  it("sends final report on unmount", () => {
    const { unmount } = renderHook(() => useProgressReporter("s1", () => 9000));

    unmount();
    expect(reportProgressMock).toHaveBeenCalledWith(expect.objectContaining({
      sessionId: "s1",
      positionMs: "9000",
      isPaused: true,
    }));
  });

  it("stops interval on unmount", () => {
    const { unmount } = renderHook(() => useProgressReporter("s1", () => 0));
    unmount();
    reportProgressMock.mockClear();

    vi.advanceTimersByTime(30_000);
    expect(reportProgressMock).not.toHaveBeenCalled();
  });
});
