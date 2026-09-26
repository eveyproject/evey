import { describe, it, expect, vi, beforeEach } from "vitest";
import { act, render } from "@testing-library/react";
import { useRef } from "react";
import { AutoMarkWatched } from "./AutoMarkWatched";

const mockMutate = vi.fn();
let mockProgress: { played?: boolean } | undefined;

vi.mock("@tanstack/react-query", () => ({
  useQueryClient: () => ({ invalidateQueries: vi.fn() }),
}));

vi.mock("@evey/api", () => ({
  usePlaybackServiceMarkAsWatched: (opts?: { mutation?: { onSuccess?: () => void } }) => ({
    mutate: (vars: unknown) => {
      mockMutate(vars);
      opts?.mutation?.onSuccess?.();
    },
  }),
  usePlaybackServiceGetWatchProgress: () => ({ data: mockProgress }),
}));

vi.mock("@evey/store", () => ({
  useAuthStore: (selector: (s: { profileId: string | null }) => unknown) =>
    selector({ profileId: "prof-1" }),
}));

beforeEach(() => {
  mockMutate.mockClear();
  mockProgress = undefined;
});

function makeVideo(duration: number, currentTime: number): HTMLVideoElement {
  const v = document.createElement("video");
  Object.defineProperty(v, "duration", { value: duration, configurable: true });
  Object.defineProperty(v, "currentTime", { value: currentTime, writable: true, configurable: true });
  return v;
}

function fireTick(video: HTMLVideoElement) {
  act(() => { video.dispatchEvent(new Event("timeupdate")); });
}

describe("AutoMarkWatched", () => {
  it("does not fire below 90% threshold", () => {
    const video = makeVideo(100, 50);
    function H() {
      const ref = useRef(video);
      return <AutoMarkWatched mediaId="m-1" videoRef={ref} />;
    }
    render(<H />);
    fireTick(video);
    expect(mockMutate).not.toHaveBeenCalled();
  });

  it("fires once when crossing 90%", () => {
    const video = makeVideo(100, 91);
    function H() {
      const ref = useRef(video);
      return <AutoMarkWatched mediaId="m-1" videoRef={ref} />;
    }
    render(<H />);
    fireTick(video);
    expect(mockMutate).toHaveBeenCalledTimes(1);
    expect(mockMutate).toHaveBeenCalledWith({ mediaItemId: "m-1", params: { profileId: "prof-1" } });
  });

  it("does not double-fire on subsequent ticks", () => {
    const video = makeVideo(100, 95);
    function H() {
      const ref = useRef(video);
      return <AutoMarkWatched mediaId="m-1" videoRef={ref} />;
    }
    render(<H />);
    fireTick(video);
    fireTick(video);
    fireTick(video);
    expect(mockMutate).toHaveBeenCalledTimes(1);
  });

  it("skips firing if the server already reports played", () => {
    mockProgress = { played: true };
    const video = makeVideo(100, 95);
    function H() {
      const ref = useRef(video);
      return <AutoMarkWatched mediaId="m-1" videoRef={ref} />;
    }
    render(<H />);
    fireTick(video);
    expect(mockMutate).not.toHaveBeenCalled();
  });

  it("uses knownDurationSec when video.duration is missing", () => {
    const video = makeVideo(NaN, 92);
    function H() {
      const ref = useRef(video);
      return <AutoMarkWatched mediaId="m-1" videoRef={ref} knownDurationSec={100} />;
    }
    render(<H />);
    fireTick(video);
    expect(mockMutate).toHaveBeenCalledTimes(1);
  });

  it("ignores when both duration sources are zero/NaN", () => {
    const video = makeVideo(NaN, 50);
    function H() {
      const ref = useRef(video);
      return <AutoMarkWatched mediaId="m-1" videoRef={ref} />;
    }
    render(<H />);
    fireTick(video);
    expect(mockMutate).not.toHaveBeenCalled();
  });

  // Anti-regression: with transcode mode, video.currentTime is local to the
  // current stream (resets to ~0 on every seek-restart). The auto-mark
  // threshold must compare against absolute position = transcodeOffset +
  // currentTime, otherwise a user who seeks to 80% and plays 12% locally
  // (= 92% absolute) never trips the threshold.
  it("uses transcodeOffsetRef to compute absolute position", () => {
    const video = makeVideo(100, 12); // local time 12s
    function H() {
      const ref = useRef(video);
      const offsetRef = useRef(80); // started transcoding at 80s
      return <AutoMarkWatched mediaId="m-1" videoRef={ref} transcodeOffsetRef={offsetRef} />;
    }
    render(<H />);
    fireTick(video);
    // absolute = 80 + 12 = 92, threshold = 100 * 0.9 = 90 → fires.
    expect(mockMutate).toHaveBeenCalledTimes(1);
  });

  it("transcode local time below absolute threshold does not fire", () => {
    const video = makeVideo(100, 5); // local time 5s
    function H() {
      const ref = useRef(video);
      const offsetRef = useRef(80); // absolute = 85, below 90
      return <AutoMarkWatched mediaId="m-1" videoRef={ref} transcodeOffsetRef={offsetRef} />;
    }
    render(<H />);
    fireTick(video);
    expect(mockMutate).not.toHaveBeenCalled();
  });
});
