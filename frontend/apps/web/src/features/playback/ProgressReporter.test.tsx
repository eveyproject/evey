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
      castDeviceName: "",
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

  it("backs off after consecutive errors - only reports every 3rd tick", async () => {
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

  it("reports the CAST playback instead of the paused local video while a cast is active", async () => {
    const { ProgressReporter } = await import("./ProgressReporter");
    // Local video paused where the user handed off to the Chromecast.
    const videoRef = createVideoRef({ paused: true, currentTime: 42 } as Partial<HTMLVideoElement>);

    render(
      React.createElement(ProgressReporter, {
        sessionId: "sess1",
        videoRef,
        cast: {
          active: true,
          positionSec: 1500.5,
          isPaused: false,
          volume: 0.5,
          isMuted: true,
          deviceName: "Salon TV",
          playPause: vi.fn(),
          stop: vi.fn(),
        },
      }),
    );

    await vi.advanceTimersByTimeAsync(3_000);
    expect(mockReportProgress).toHaveBeenCalledWith({
      sessionId: "sess1",
      positionMs: "1500500",
      isPaused: false,
      isMuted: true,
      volumeLevel: 50,
      castDeviceName: "Salon TV",
    });
  });

  it("reports the local video with an EMPTY castDeviceName when not casting (clears a previous relay)", async () => {
    const { ProgressReporter } = await import("./ProgressReporter");
    const videoRef = createVideoRef();

    render(
      React.createElement(ProgressReporter, { sessionId: "sess1", videoRef }),
    );

    await vi.advanceTimersByTimeAsync(3_000);
    expect(mockReportProgress).toHaveBeenCalledWith(
      expect.objectContaining({ positionMs: "42000", castDeviceName: "" }),
    );
  });

  it("routes admin remote commands to the cast session while casting", async () => {
    const { ProgressReporter } = await import("./ProgressReporter");
    const playPause = vi.fn();
    const stop = vi.fn();
    const onRemoteStop = vi.fn();
    mockReportProgress.mockResolvedValue({ remoteCommand: "pause" });

    const videoRef = createVideoRef({ paused: true } as Partial<HTMLVideoElement>);
    const pauseSpy = vi.spyOn(videoRef.current, "pause");

    render(
      React.createElement(ProgressReporter, {
        sessionId: "sess1",
        videoRef,
        onRemoteStop,
        cast: {
          active: true,
          positionSec: 100,
          isPaused: false,
          volume: 1,
          isMuted: false,
          deviceName: "Salon TV",
          playPause,
          stop,
        },
      }),
    );

    await vi.advanceTimersByTimeAsync(3_000);
    expect(playPause).toHaveBeenCalledTimes(1);
    expect(pauseSpy).not.toHaveBeenCalled();

    mockReportProgress.mockResolvedValue({ remoteCommand: "stop" });
    await vi.advanceTimersByTimeAsync(3_000);
    expect(stop).toHaveBeenCalledTimes(1);
    expect(onRemoteStop).toHaveBeenCalledTimes(1);
  });

  it("hands an admin message to the player instead of a global toast", async () => {
    const { ProgressReporter } = await import("./ProgressReporter");
    const { toast } = await import("@evey/store");
    const onRemoteMessage = vi.fn();
    mockReportProgress.mockResolvedValue({ remoteCommand: "message:Dinner in 5" });

    render(
      React.createElement(ProgressReporter, {
        sessionId: "sess1",
        videoRef: createVideoRef(),
        onRemoteMessage,
      }),
    );

    await vi.advanceTimersByTimeAsync(3_000);
    expect(onRemoteMessage).toHaveBeenCalledWith("Dinner in 5");
    expect(toast.info).not.toHaveBeenCalled();
  });

  it("falls back to a toast for an admin message when the player gives no handler", async () => {
    const { ProgressReporter } = await import("./ProgressReporter");
    const { toast } = await import("@evey/store");
    mockReportProgress.mockResolvedValue({ remoteCommand: "message:Dinner in 5" });

    render(React.createElement(ProgressReporter, { sessionId: "sess1", videoRef: createVideoRef() }));

    await vi.advanceTimersByTimeAsync(3_000);
    expect(toast.info).toHaveBeenCalledWith("Dinner in 5");
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

  // The player unmounts as a whole: React has already cleared videoRef when
  // the effect cleans up, so the final report must come from what was seen
  // while the element still held the media (measured: 0 calls before 27-09).
  describe("final report when the player closes", () => {
    function Host({ sessionId, onVideo }: { sessionId: string; onVideo: (v: HTMLVideoElement) => void }) {
      const ref = React.useRef<HTMLVideoElement | null>(null);
      return React.createElement(React.Fragment, null,
        React.createElement("video", { ref: (el: HTMLVideoElement | null) => { ref.current = el; if (el) onVideo(el); } }),
        React.createElement(ProgressReporterRef.current!, { sessionId, videoRef: ref }),
      );
    }
    const ProgressReporterRef: { current: null | typeof import("./ProgressReporter").ProgressReporter } = { current: null };
    function play(v: HTMLVideoElement, at: number, readyState = 4) {
      Object.defineProperty(v, "readyState", { value: readyState, configurable: true });
      Object.defineProperty(v, "currentTime", { value: at, configurable: true, writable: true });
      Object.defineProperty(v, "paused", { value: false, configurable: true });
      Object.defineProperty(v, "muted", { value: false, configurable: true });
      Object.defineProperty(v, "volume", { value: 1, configurable: true });
      v.dispatchEvent(new Event("timeupdate"));
    }

    beforeEach(async () => {
      ProgressReporterRef.current = (await import("./ProgressReporter")).ProgressReporter;
    });

    it("reports the last position seen when the whole player unmounts", () => {
      let video: HTMLVideoElement | null = null;
      const { unmount } = render(React.createElement(Host, { sessionId: "sess1", onVideo: (v) => { video = v; } }));
      play(video!, 1234.5);
      unmount();
      expect(mockReportProgress).toHaveBeenCalledTimes(1);
      expect(mockReportProgress).toHaveBeenCalledWith(expect.objectContaining({ sessionId: "sess1", positionMs: "1234500", castDeviceName: "" }));
    });

    it("keeps the last real position when the element is emptied before teardown", () => {
      let video: HTMLVideoElement | null = null;
      const { unmount } = render(React.createElement(Host, { sessionId: "sess1", onVideo: (v) => { video = v; } }));
      play(video!, 600);
      // hls.destroy detaches the media: no source, the clock reads 0.
      play(video!, 0, 0);
      unmount();
      expect(mockReportProgress).toHaveBeenCalledWith(expect.objectContaining({ positionMs: "600000" }));
    });

    it("sends nothing when no media was ever loaded", () => {
      const { unmount } = render(React.createElement(Host, { sessionId: "sess1", onVideo: () => {} }));
      unmount();
      expect(mockReportProgress).not.toHaveBeenCalled();
    });

    it("closes the previous session with its own position when the session changes", () => {
      let video: HTMLVideoElement | null = null;
      const onVideo = (v: HTMLVideoElement) => { video = v; };
      const { rerender } = render(React.createElement(Host, { sessionId: "sess1", onVideo }));
      play(video!, 2400);
      rerender(React.createElement(Host, { sessionId: "sess2", onVideo }));
      expect(mockReportProgress).toHaveBeenCalledWith(expect.objectContaining({ sessionId: "sess1", positionMs: "2400000" }));
    });
  });
});
