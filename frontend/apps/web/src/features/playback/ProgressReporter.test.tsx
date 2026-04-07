import React from "react";
import { render } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

const mockReportProgress = vi.fn().mockResolvedValue({});

vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
  playbackServiceReportProgress: (...args: unknown[]) => mockReportProgress(...args),
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
}));

function createVideoRef(overrides: Partial<HTMLVideoElement> = {}) {
  const video = document.createElement("video");
  Object.defineProperty(video, "paused", { value: false, writable: true });
  Object.defineProperty(video, "currentTime", { value: 42, writable: true });
  Object.defineProperty(video, "muted", { value: false, writable: true });
  Object.defineProperty(video, "volume", { value: 0.8, writable: true });
  Object.assign(video, overrides);
  return { current: video };
}

describe("ProgressReporter", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
    mockReportProgress.mockResolvedValue({});
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("renders null (no DOM output)", async () => {
    const { ProgressReporter } = await import("./ProgressReporter");
    const { container } = render(
      React.createElement(ProgressReporter, {
        sessionId: "sess1",
        videoRef: createVideoRef(),
      }),
    );
    expect(container.firstChild).toBeNull();
  });

  it("reports progress every 3 seconds when video is playing", async () => {
    const { ProgressReporter } = await import("./ProgressReporter");
    const videoRef = createVideoRef();

    render(
      React.createElement(ProgressReporter, { sessionId: "sess1", videoRef }),
    );

    await vi.advanceTimersByTimeAsync(3_000);
    expect(mockReportProgress).toHaveBeenCalledTimes(1);
    expect(mockReportProgress).toHaveBeenCalledWith({
      sessionId: "sess1",
      positionMs: "42000",
      isPaused: false,
      isMuted: false,
      volumeLevel: 80,
    });

    await vi.advanceTimersByTimeAsync(3_000);
    expect(mockReportProgress).toHaveBeenCalledTimes(2);
  });

  it("does not report when videoRef.current is null", async () => {
    const { ProgressReporter } = await import("./ProgressReporter");
    const videoRef = { current: null };

    render(
      React.createElement(ProgressReporter, { sessionId: "sess1", videoRef }),
    );

    await vi.advanceTimersByTimeAsync(9_000);
    expect(mockReportProgress).not.toHaveBeenCalled();
  });

  it("skips tick when a request is already pending", async () => {
    const { ProgressReporter } = await import("./ProgressReporter");
    let resolveFirst: () => void;
    mockReportProgress.mockImplementationOnce(() => new Promise<Record<string, never>>((r) => { resolveFirst = () => r({}); }));

    const videoRef = createVideoRef();
    render(
      React.createElement(ProgressReporter, { sessionId: "sess1", videoRef }),
    );

    await vi.advanceTimersByTimeAsync(3_000);
    expect(mockReportProgress).toHaveBeenCalledTimes(1);

    await vi.advanceTimersByTimeAsync(3_000);
    expect(mockReportProgress).toHaveBeenCalledTimes(1);

    resolveFirst!();
    await vi.advanceTimersByTimeAsync(0);

    await vi.advanceTimersByTimeAsync(3_000);
    expect(mockReportProgress).toHaveBeenCalledTimes(2);
  });

  it("backs off after consecutive errors — only reports every 3rd tick", async () => {
    const { ProgressReporter } = await import("./ProgressReporter");
    mockReportProgress.mockRejectedValue(new Error("network"));

    const videoRef = createVideoRef();
    render(
      React.createElement(ProgressReporter, { sessionId: "sess1", videoRef }),
    );

    // tick 1: calls, rejects, errorCount becomes 1
    await vi.advanceTimersByTimeAsync(3_000);
    expect(mockReportProgress).toHaveBeenCalledTimes(1);

    // tick 2: errorCount=1, 1%3 !== 0, skip (errorCount becomes 2)
    await vi.advanceTimersByTimeAsync(3_000);
    expect(mockReportProgress).toHaveBeenCalledTimes(1);

    // tick 3: errorCount=2, 2%3 !== 0, skip (errorCount becomes 3)
    await vi.advanceTimersByTimeAsync(3_000);
    expect(mockReportProgress).toHaveBeenCalledTimes(1);

    // tick 4: errorCount=3, 3%3 === 0, report
    await vi.advanceTimersByTimeAsync(3_000);
    expect(mockReportProgress).toHaveBeenCalledTimes(2);
  });

  it("clears interval on unmount", async () => {
    const clearSpy = vi.spyOn(globalThis, "clearInterval");

    const { ProgressReporter } = await import("./ProgressReporter");
    const videoRef = createVideoRef();
    const { unmount } = render(
      React.createElement(ProgressReporter, { sessionId: "sess1", videoRef }),
    );

    unmount();
    expect(clearSpy).toHaveBeenCalled();
  });
});
