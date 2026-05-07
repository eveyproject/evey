import { describe, it, expect, vi, beforeEach } from "vitest";
import { render } from "@testing-library/react";
import { useRef } from "react";
import { AutoMarkWatched } from "./AutoMarkWatched";

const mockMutate = vi.fn();
let mockProgress: { played?: boolean } | undefined;

vi.mock("@evey/api", () => ({
  usePlaybackServiceMarkAsWatched: () => ({ mutate: mockMutate }),
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
  video.dispatchEvent(new Event("timeupdate"));
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
});
