import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";

const reportProgressMock = vi.fn();
vi.mock("@evey/api", () => ({
  playbackServiceReportProgress: (...args: unknown[]) => reportProgressMock(...args),
}));

const toastInfoMock = vi.fn();
vi.mock("@evey/store", () => ({
  toast: { info: (...args: unknown[]) => toastInfoMock(...args) },
}));

import { useProgressReporter } from "./useProgressReporter";

beforeEach(() => {
  vi.useFakeTimers();
  reportProgressMock.mockReset();
  toastInfoMock.mockReset();
  reportProgressMock.mockResolvedValue(undefined);
});
afterEach(() => { vi.useRealTimers(); });

describe("useProgressReporter", () => {
  it("does not report when sessionId is null", () => {
    renderHook(() => useProgressReporter(null, () => 0));
    vi.advanceTimersByTime(10_000);
    expect(reportProgressMock).not.toHaveBeenCalled();
  });

  it("reports position every 3s", async () => {
    let pos = 5000;
    renderHook(() => useProgressReporter("s1", () => pos));

    await act(async () => { vi.advanceTimersByTime(3_000); });
    expect(reportProgressMock).toHaveBeenCalledTimes(1);
    expect(reportProgressMock).toHaveBeenCalledWith({
      sessionId: "s1",
      positionMs: "5000",
      isPaused: false,
      isMuted: false,
      volumeLevel: 100,
    });

    pos = 15000;
    await act(async () => { vi.advanceTimersByTime(3_000); });
    expect(reportProgressMock).toHaveBeenCalledTimes(2);
    expect(reportProgressMock).toHaveBeenLastCalledWith(expect.objectContaining({
      positionMs: "15000",
    }));
  });

  it("reports even when paused (includes isPaused=true)", async () => {
    renderHook(() => useProgressReporter("s1", () => 1000, () => true));

    await act(async () => { vi.advanceTimersByTime(3_000); });
    expect(reportProgressMock).toHaveBeenCalledWith(expect.objectContaining({
      isPaused: true,
    }));
  });

  it("includes volume and muted state", async () => {
    renderHook(() =>
      useProgressReporter("s1", () => 3000, () => false, () => 0.5, () => true),
    );

    await act(async () => { vi.advanceTimersByTime(3_000); });
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

  it("calls onRemotePause on pause command", async () => {
    const onPause = vi.fn();
    reportProgressMock.mockResolvedValue({ remoteCommand: "pause" });

    renderHook(() => useProgressReporter("s1", () => 0, undefined, undefined, undefined, {
      onRemotePause: onPause,
    }));

    await act(async () => { vi.advanceTimersByTime(3_000); });
    expect(onPause).toHaveBeenCalledTimes(1);
  });

  it("calls onRemotePlay on play command", async () => {
    const onPlay = vi.fn();
    reportProgressMock.mockResolvedValue({ remoteCommand: "play" });

    renderHook(() => useProgressReporter("s1", () => 0, undefined, undefined, undefined, {
      onRemotePlay: onPlay,
    }));

    await act(async () => { vi.advanceTimersByTime(3_000); });
    expect(onPlay).toHaveBeenCalledTimes(1);
  });

  it("calls onRemotePause then onRemoteStop on stop command", async () => {
    const onPause = vi.fn();
    const onStop = vi.fn();
    reportProgressMock.mockResolvedValue({ remoteCommand: "stop" });

    renderHook(() => useProgressReporter("s1", () => 0, undefined, undefined, undefined, {
      onRemotePause: onPause,
      onRemoteStop: onStop,
    }));

    await act(async () => { vi.advanceTimersByTime(3_000); });
    expect(onPause).toHaveBeenCalledTimes(1);
    expect(onStop).toHaveBeenCalledTimes(1);
  });

  it("shows toast on message command", async () => {
    reportProgressMock.mockResolvedValue({ remoteCommand: "message:Hello world" });

    renderHook(() => useProgressReporter("s1", () => 0));

    await act(async () => { vi.advanceTimersByTime(3_000); });
    expect(toastInfoMock).toHaveBeenCalledWith("Hello world");
  });

  it("backs off on errors (skip 2 ticks after failure)", async () => {
    reportProgressMock.mockRejectedValueOnce(new Error("fail"));

    renderHook(() => useProgressReporter("s1", () => 0));

    // tick 1: fails
    await act(async () => { vi.advanceTimersByTime(3_000); });
    expect(reportProgressMock).toHaveBeenCalledTimes(1);

    // tick 2: skipped (errorCount=1, 1%3 !== 0)
    reportProgressMock.mockResolvedValue(undefined);
    await act(async () => { vi.advanceTimersByTime(3_000); });
    expect(reportProgressMock).toHaveBeenCalledTimes(1);

    // tick 3: skipped (errorCount=2, 2%3 !== 0)
    await act(async () => { vi.advanceTimersByTime(3_000); });
    expect(reportProgressMock).toHaveBeenCalledTimes(1);

    // tick 4: retries (errorCount=3, 3%3 === 0)
    await act(async () => { vi.advanceTimersByTime(3_000); });
    expect(reportProgressMock).toHaveBeenCalledTimes(2);
  });
});
