import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, waitFor, act } from "@testing-library/react";
import React from "react";

// The real parser is exercised by its own tests; here the blob is a stand-in
// and the magic check decides text-vs-bitmap.
const pgs = vi.hoisted(() => ({ isPGS: true, onlyFirstCue: false }));
vi.mock("@evey/api", () => ({
  isPGSBlob: () => pgs.isPGS,
  parsePGSBlob: () => ({
    screenW: 1920,
    screenH: 1080,
    cues: [
      { startMs: 1000, endMs: 2000, x: 192, y: 810, w: 960, h: 108, forced: false, png: new Uint8Array([1]) },
      { startMs: 5000, endMs: 6000, x: 0, y: 0, w: 1920, h: 540, forced: true, png: new Uint8Array([2]) },
    ].slice(0, pgs.onlyFirstCue ? 1 : 2),
  }),
  findActivePGSCue: (cues: { startMs: number; endMs: number }[], t: number) =>
    cues.findIndex((c) => t >= c.startMs && t < c.endMs),
}));

import { PGSSubtitleOverlay } from "./PGSSubtitleOverlay";

const fetchMock = vi.fn();
let objectUrls = 0;

function mount(props: Partial<React.ComponentProps<typeof PGSSubtitleOverlay>> = {}) {
  const video = document.createElement("video");
  Object.defineProperty(video, "currentTime", { value: 0, writable: true });
  const videoRef = { current: video };
  const utils = render(
    <PGSSubtitleOverlay mediaId={props.mediaId ?? "m1"} trackIndex={props.trackIndex === undefined ? 0 : props.trackIndex} streamToken={props.streamToken ?? "tok"} videoRef={videoRef} offsetMs={props.offsetMs} />,
  );
  return { video, ...utils };
}

function seekTo(video: HTMLVideoElement, sec: number) {
  (video as unknown as { currentTime: number }).currentTime = sec;
  act(() => {
    video.dispatchEvent(new Event("timeupdate"));
  });
}

beforeEach(() => {
  pgs.isPGS = true;
  pgs.onlyFirstCue = false;
  objectUrls = 0;
  fetchMock.mockReset();
  fetchMock.mockResolvedValue({ ok: true, arrayBuffer: async () => new ArrayBuffer(8) });
  vi.stubGlobal("fetch", fetchMock);
  URL.createObjectURL = vi.fn(() => `blob:cue-${objectUrls++}`);
  URL.revokeObjectURL = vi.fn();
});
afterEach(() => vi.unstubAllGlobals());

// The track is fetched once with the stream token, drawn only when the blob
// really is a bitmap track, positioned as percentages of the SUBTITLE plane,
// follows the playhead (shifted by the viewer's resync) off the video's own
// events, and a track already loaded is never fetched again.
describe("PGSSubtitleOverlay", () => {
  // Switching from a full track to a forced one with fewer cues while a late
  // cue is on screen used to render the new track with the old index:
  // cues[active] was undefined and the player crashed on cue.x.
  it("switching to a track with fewer cues mid-cue neither crashes nor keeps the old image", async () => {
    const video = document.createElement("video");
    Object.defineProperty(video, "currentTime", { value: 0, writable: true });
    const videoRef = { current: video };
    const el = (trackIndex: number) => (
      <PGSSubtitleOverlay mediaId="m-switch" trackIndex={trackIndex} streamToken="tok" videoRef={videoRef} />
    );
    const { container, rerender } = render(el(0));
    seekTo(video, 5.2);
    await waitFor(() => expect(container.querySelector("img")?.getAttribute("src")).toBe("blob:cue-1"));

    pgs.onlyFirstCue = true;
    rerender(el(1));
    expect(container.querySelector("img")).toBeNull();
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    await act(async () => {});
    expect(container.querySelector("img")).toBeNull();
    seekTo(video, 1.5);
    expect(container.querySelector("img")?.getAttribute("src")).toBe("blob:cue-2");
  });

  it("draws the active cue as percentages of the subtitle plane and follows the playhead", async () => {
    const { video, container } = mount({ mediaId: "m-draw" });
    expect(container.querySelector("img")).toBeNull();
    seekTo(video, 1.5);
    await waitFor(() => expect(container.querySelector("img")).toBeTruthy());
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/media/m-draw/subtitle/0?token=tok");
    const img = container.querySelector("img") as HTMLImageElement;
    expect(img.getAttribute("src")).toBe("blob:cue-0");
    expect(img.style.left).toBe("10%");
    expect(img.style.top).toBe("75%");
    expect(img.style.width).toBe("50%");
    expect(img.style.height).toBe("10%");

    seekTo(video, 3);
    expect(container.querySelector("img")).toBeNull();
    seekTo(video, 5.2);
    expect(container.querySelector("img")?.getAttribute("src")).toBe("blob:cue-1");
  });

  it("applies the viewer's resync offset and reuses a loaded track without refetching", async () => {
    const first = mount({ mediaId: "m-cache", offsetMs: 1000 });
    // With +1000 ms the cue at 1-2 s is shown when the playhead is at 2.5 s.
    seekTo(first.video, 2.5);
    await waitFor(() => expect(first.container.querySelector("img")).toBeTruthy());
    seekTo(first.video, 1.5);
    expect(first.container.querySelector("img")).toBeNull();
    first.unmount();

    const again = mount({ mediaId: "m-cache" });
    seekTo(again.video, 1.5);
    expect(again.container.querySelector("img")?.getAttribute("src")).toBe("blob:cue-0");
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("draws nothing for a text track answered by the same route, nor without a track", async () => {
    pgs.isPGS = false;
    const { video, container } = mount({ mediaId: "m-text" });
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    await act(async () => {});
    seekTo(video, 1.5);
    expect(container.querySelector("img")).toBeNull();

    mount({ mediaId: "m-none", trackIndex: null });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  // A binge must not keep every track's blobs alive for the life of the tab:
  // past the cap the oldest track is dropped and its object URLs revoked, and
  // coming back to it fetches again instead of drawing a revoked URL.
  it("evicts the oldest cached track and revokes its object URLs past the cap", async () => {
    const revoke = URL.revokeObjectURL as ReturnType<typeof vi.fn>;
    const ids = ["m-evict-1", "m-evict-2", "m-evict-3", "m-evict-4"];
    for (const mediaId of ids) {
      const { video, container, unmount } = mount({ mediaId });
      seekTo(video, 1.5);
      await waitFor(() => expect(container.querySelector("img")).toBeTruthy());
      unmount();
    }
    expect(fetchMock).toHaveBeenCalledTimes(4);
    // Each track built 2 URLs; only the first track's pair may be gone. The
    // earlier tests' tracks were evicted along the way, which is the point.
    const firstPair = (URL.createObjectURL as ReturnType<typeof vi.fn>).mock.results.slice(0, 2).map((r) => r.value);
    for (const u of firstPair) expect(revoke).toHaveBeenCalledWith(u);
    const lastPair = (URL.createObjectURL as ReturnType<typeof vi.fn>).mock.results.slice(-2).map((r) => r.value);
    for (const u of lastPair) expect(revoke).not.toHaveBeenCalledWith(u);

    const back = mount({ mediaId: ids[0] });
    seekTo(back.video, 1.5);
    await waitFor(() => expect(back.container.querySelector("img")).toBeTruthy());
    expect(fetchMock).toHaveBeenCalledTimes(5);
    expect(back.container.querySelector("img")?.getAttribute("src")).not.toBe(firstPair[0]);
  });
});
