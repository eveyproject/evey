import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";

const reportProgressMock = vi.fn();
const eveyFetchMock = vi.fn();
vi.mock("@evey/api", () => ({
  playbackServiceReportProgress: (...args: unknown[]) => reportProgressMock(...args),
  eveyFetch: (...args: unknown[]) => eveyFetchMock(...args),
}));

const toastInfoMock = vi.fn();
vi.mock("@evey/store", () => ({
  toast: { info: (...args: unknown[]) => toastInfoMock(...args) },
}));

import { useProgressReporter, shouldTriggerPrewarm, PREWARM_PROGRESS_THRESHOLD } from "./useProgressReporter";

beforeEach(() => {
  vi.useFakeTimers();
  reportProgressMock.mockReset();
  eveyFetchMock.mockReset();
  toastInfoMock.mockReset();
  reportProgressMock.mockResolvedValue(undefined);
  eveyFetchMock.mockResolvedValue(undefined);
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
      castDeviceName: "",
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

  it("keeps the last known position when the released player reads 0 on unmount", async () => {
    let released = false;
    const { unmount } = renderHook(() => useProgressReporter("s1", () => (released ? 0 : 267_000)));
    await act(async () => { vi.advanceTimersByTime(3_000); });
    released = true;
    reportProgressMock.mockClear();

    unmount();
    expect(reportProgressMock).toHaveBeenCalledWith(expect.objectContaining({ sessionId: "s1", positionMs: "267000" }));
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
    // Held as long as the web's banner and the cast receiver (12 s).
    expect(toastInfoMock).toHaveBeenCalledWith("Hello world", { duration: 12_000 });
  });

  // While casting, the report must name the cast device: the server withholds
  // remote commands from a casting reporter so the TV's own beacon consumes
  // them - without this field the phone ate the admin's message as a local
  // toast and the TV never saw it.
  it("includes the cast device name in periodic reports while casting", async () => {
    renderHook(() =>
      useProgressReporter("s1", () => 0, undefined, undefined, undefined, undefined, undefined, () => "Salon TV"),
    );

    await act(async () => { vi.advanceTimersByTime(3_000); });
    expect(reportProgressMock).toHaveBeenCalledWith(expect.objectContaining({
      castDeviceName: "Salon TV",
    }));
  });

  it("includes the cast device name in the final unmount report", async () => {
    const { unmount } = renderHook(() =>
      useProgressReporter("s1", () => 0, undefined, undefined, undefined, undefined, undefined, () => "Salon TV"),
    );

    unmount();
    expect(reportProgressMock).toHaveBeenCalledWith(expect.objectContaining({
      castDeviceName: "Salon TV",
    }));
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

describe("shouldTriggerPrewarm (pure rule)", () => {
  const base = { mediaId: "m1", isEpisode: true, prewarmedMediaId: null as string | null };

  it("is false strictly below the threshold", () => {
    expect(shouldTriggerPrewarm({ ...base, positionMs: 8499, durationMs: 10_000 })).toBe(false);
  });

  it("is true exactly at the threshold", () => {
    expect(PREWARM_PROGRESS_THRESHOLD).toBe(0.85);
    expect(shouldTriggerPrewarm({ ...base, positionMs: 8500, durationMs: 10_000 })).toBe(true);
  });

  it("is true above the threshold", () => {
    expect(shouldTriggerPrewarm({ ...base, positionMs: 9500, durationMs: 10_000 })).toBe(true);
  });

  it("is false after a rewind back below the threshold", () => {
    expect(shouldTriggerPrewarm({ ...base, positionMs: 2000, durationMs: 10_000 })).toBe(false);
  });

  it("is false once already prewarmed for this media, even if the user rewinds and crosses the threshold again", () => {
    expect(shouldTriggerPrewarm({ ...base, positionMs: 9500, durationMs: 10_000, prewarmedMediaId: "m1" })).toBe(false);
  });

  it("is true again once the media item changes", () => {
    expect(shouldTriggerPrewarm({ ...base, positionMs: 9500, durationMs: 10_000, prewarmedMediaId: "m0" })).toBe(true);
  });

  it("is false without a mediaId", () => {
    expect(shouldTriggerPrewarm({ ...base, mediaId: undefined, positionMs: 9500, durationMs: 10_000 })).toBe(false);
  });

  it("is false when not an episode", () => {
    expect(shouldTriggerPrewarm({ ...base, isEpisode: false, positionMs: 9500, durationMs: 10_000 })).toBe(false);
  });

  it("is false when duration is unknown", () => {
    expect(shouldTriggerPrewarm({ ...base, positionMs: 9500, durationMs: 0 })).toBe(false);
  });
});

describe("useProgressReporter prewarm-next wiring", () => {
  it("POSTs prewarm-next exactly once when crossing the threshold, never again for the same media", async () => {
    let pos = 8600; // 86% of a 10s (10_000ms) episode
    renderHook(() =>
      useProgressReporter("s1", () => pos, undefined, undefined, undefined, undefined, {
        mediaId: "ep1",
        isEpisode: true,
        getDurationMs: () => 10_000,
      }),
    );

    await act(async () => { vi.advanceTimersByTime(3_000); });
    expect(eveyFetchMock).toHaveBeenCalledTimes(1);
    expect(eveyFetchMock).toHaveBeenCalledWith({ url: "/api/v1/media/ep1/prewarm-next", method: "POST" });

    // Every subsequent tick (progress keeps climbing) must not re-fire -
    // a POST per tick would hammer the transcoder on weak hardware.
    pos = 9200;
    await act(async () => { vi.advanceTimersByTime(3_000); });
    pos = 9800;
    await act(async () => { vi.advanceTimersByTime(3_000); });
    expect(eveyFetchMock).toHaveBeenCalledTimes(1);
  });

  it("does not POST while under the threshold", async () => {
    const pos = 1000;
    renderHook(() =>
      useProgressReporter("s1", () => pos, undefined, undefined, undefined, undefined, {
        mediaId: "ep1",
        isEpisode: true,
        getDurationMs: () => 10_000,
      }),
    );

    await act(async () => { vi.advanceTimersByTime(9_000); });
    expect(eveyFetchMock).not.toHaveBeenCalled();
  });

  it("fires again for a new media item after already firing once for the previous one", async () => {
    let mediaId = "ep1";
    let pos = 8600;
    const { rerender } = renderHook(
      ({ id, position }: { id: string; position: number }) =>
        useProgressReporter("s1", () => position, undefined, undefined, undefined, undefined, {
          mediaId: id,
          isEpisode: true,
          getDurationMs: () => 10_000,
        }),
      { initialProps: { id: mediaId, position: pos } },
    );

    await act(async () => { vi.advanceTimersByTime(3_000); });
    expect(eveyFetchMock).toHaveBeenCalledTimes(1);

    mediaId = "ep2";
    pos = 8700;
    rerender({ id: mediaId, position: pos });
    await act(async () => { vi.advanceTimersByTime(3_000); });
    expect(eveyFetchMock).toHaveBeenCalledTimes(2);
    expect(eveyFetchMock).toHaveBeenLastCalledWith({ url: "/api/v1/media/ep2/prewarm-next", method: "POST" });
  });
});
