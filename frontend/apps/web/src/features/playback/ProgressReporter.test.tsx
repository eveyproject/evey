import React from "react";
import { render } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

const mockMutate = vi.fn();
const mockReportProgress = {
  mutate: mockMutate,
  isPending: false,
  isError: false,
};

vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
  usePlaybackServiceReportProgress: () => mockReportProgress,
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
    mockReportProgress.isPending = false;
    mockReportProgress.isError = false;
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

  it("reports progress every 10 seconds when video is playing", async () => {
    const { ProgressReporter } = await import("./ProgressReporter");
    const videoRef = createVideoRef();

    render(
      React.createElement(ProgressReporter, { sessionId: "sess1", videoRef }),
    );

    vi.advanceTimersByTime(10_000);
    expect(mockMutate).toHaveBeenCalledTimes(1);
    expect(mockMutate).toHaveBeenCalledWith({
      data: {
        sessionId: "sess1",
        positionMs: "42000",
        isPaused: false,
        isMuted: false,
        volumeLevel: 80,
      },
    });

    vi.advanceTimersByTime(10_000);
    expect(mockMutate).toHaveBeenCalledTimes(2);
  });

  it("does not report when video is paused", async () => {
    const { ProgressReporter } = await import("./ProgressReporter");
    const videoRef = createVideoRef();
    Object.defineProperty(videoRef.current, "paused", { value: true, writable: true });

    render(
      React.createElement(ProgressReporter, { sessionId: "sess1", videoRef }),
    );

    vi.advanceTimersByTime(30_000);
    expect(mockMutate).not.toHaveBeenCalled();
  });

  it("skips tick when a request is already pending", async () => {
    const { ProgressReporter } = await import("./ProgressReporter");
    mockReportProgress.isPending = true;

    const videoRef = createVideoRef();
    render(
      React.createElement(ProgressReporter, { sessionId: "sess1", videoRef }),
    );

    vi.advanceTimersByTime(10_000);
    expect(mockMutate).not.toHaveBeenCalled();
  });

  it("backs off after consecutive errors — only reports every 3rd tick", async () => {
    const { ProgressReporter } = await import("./ProgressReporter");
    mockReportProgress.isError = true;

    const videoRef = createVideoRef();
    render(
      React.createElement(ProgressReporter, { sessionId: "sess1", videoRef }),
    );

    // tick 1: errorCount becomes 1, 1%3 !== 0, skip
    vi.advanceTimersByTime(10_000);
    expect(mockMutate).toHaveBeenCalledTimes(0);

    // tick 2: errorCount becomes 2, 2%3 !== 0, skip
    vi.advanceTimersByTime(10_000);
    expect(mockMutate).toHaveBeenCalledTimes(0);

    // tick 3: errorCount becomes 3, 3%3 === 0, report
    vi.advanceTimersByTime(10_000);
    expect(mockMutate).toHaveBeenCalledTimes(1);
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
