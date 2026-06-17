import { describe, it, expect, vi, afterEach } from "vitest";
import { render, act } from "@testing-library/react";
import { PlayerProvider, usePlayerContext, usePlayerTime } from "./PlayerContext";

// Proves the perf split: a position tick (timeupdate ~4×/sec) must re-render
// ONLY the components subscribed to PlayerTimeContext (the seek bar / time
// label), not the ones reading PlayerState (the whole control bar). Before the
// split, currentTime lived in the single context value and every consumer
// re-rendered 4×/sec.

afterEach(() => vi.restoreAllMocks());

function makeVideo(initial = 0) {
  const video = document.createElement("video");
  let t = initial;
  Object.defineProperty(video, "currentTime", {
    configurable: true,
    get: () => t,
    set: (v: number) => { t = v; },
  });
  return video;
}

describe("PlayerContext time split", () => {
  it("a timeupdate re-renders usePlayerTime consumers but not usePlayerContext consumers", () => {
    const video = makeVideo(0);
    const videoRef = { current: video };

    let nowVal = 1000;
    vi.spyOn(performance, "now").mockImplementation(() => nowVal);

    let ctxRenders = 0;
    let timeRenders = 0;
    function CtxConsumer() { usePlayerContext(); ctxRenders++; return null; }
    function TimeConsumer() { const t = usePlayerTime(); timeRenders++; return <span>{t.currentTime}</span>; }

    render(
      <PlayerProvider
        videoRef={videoRef}
        audioStreams={[]}
        subtitleStreams={[]}
        currentAudioIndex={0}
        currentQuality={0}
        currentSubtitleTrackIndex={null}
      >
        <CtxConsumer />
        <TimeConsumer />
      </PlayerProvider>,
    );

    const ctxAfterMount = ctxRenders;
    const timeAfterMount = timeRenders;

    // Advance past the 250ms throttle and fire a position tick.
    act(() => {
      video.currentTime = 5;
      nowVal += 300;
      video.dispatchEvent(new Event("timeupdate"));
    });

    const msg = `ctx ${ctxAfterMount}->${ctxRenders}, time ${timeAfterMount}->${timeRenders}`;
    expect(timeRenders, msg).toBeGreaterThan(timeAfterMount); // time leaf re-rendered
    expect(ctxRenders, msg).toBe(ctxAfterMount); // control-bar consumer did NOT
  });
});
