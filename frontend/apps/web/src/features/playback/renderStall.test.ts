import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { isRenderSlowdown, isCadenceBroken, formatRenderWindow, formatCadence, createRenderStallWatch, decideRenderCap, nextLowerRung } from "./renderStall";

function win(over: Partial<Parameters<typeof isRenderSlowdown>[0]> = {}) {
  return { fps: 25, baselineFps: 25, lagMaxMs: 0, playing: true, visible: true, longIntervals: 0, maxIntervalMs: 40, medianIntervalMs: 40, skippedFrames: 0, ...over };
}

describe("isRenderSlowdown", () => {
  it("says nothing while the rate holds", () => {
    expect(isRenderSlowdown(win())).toBe(false);
    expect(isRenderSlowdown(win({ fps: 22 }))).toBe(false);
  });

  it("fires when presented frames fall well under the content's own cadence", () => {
    expect(isRenderSlowdown(win({ fps: 11 }))).toBe(true);
  });

  // A paused or backgrounded tab presents no frames and throttles timers. That
  // looks exactly like the bug and is not it.
  it("stays quiet while paused or hidden", () => {
    expect(isRenderSlowdown(win({ fps: 0, playing: false }))).toBe(false);
    expect(isRenderSlowdown(win({ fps: 0, visible: false }))).toBe(false);
  });

  it("refuses to judge before a baseline exists", () => {
    expect(isRenderSlowdown(win({ fps: 0, baselineFps: 0 }))).toBe(false);
    expect(isRenderSlowdown(win({ fps: 1, baselineFps: 4 }))).toBe(false);
  });
});

describe("formatRenderWindow", () => {
  it("stays terse: the server clamps the field it rides in", () => {
    expect(formatRenderWindow(win({ fps: 11.04, baselineFps: 25, lagMaxMs: 337.7 }))).toBe(
      "fps=11.0/25.0 lag=338ms",
    );
  });

  it("adds the hitches only when there are some", () => {
    expect(formatRenderWindow(win({ longIntervals: 3, maxIntervalMs: 83.4, skippedFrames: 1 }))).toBe(
      "fps=25.0/25.0 lag=0ms jit=3/83ms sk=1",
    );
  });
});

describe("isCadenceBroken", () => {
  it("fires on frames held too long or never shown, at a full frame count", () => {
    expect(isCadenceBroken(win({ longIntervals: 3 }))).toBe(true);
    expect(isCadenceBroken(win({ skippedFrames: 2 }))).toBe(true);
    expect(isCadenceBroken(win({ longIntervals: 1, skippedFrames: 1 }))).toBe(false);
  });

  it("leaves a slowdown, a paused player and a hidden tab to their own rules", () => {
    expect(isCadenceBroken(win({ fps: 8, longIntervals: 5 }))).toBe(false);
    expect(isCadenceBroken(win({ playing: false, longIntervals: 5 }))).toBe(false);
    expect(isCadenceBroken(win({ visible: false, longIntervals: 5 }))).toBe(false);
  });
});

describe("formatCadence", () => {
  it("carries the window and the session totals", () => {
    expect(formatCadence(win({ fps: 24, longIntervals: 3, maxIntervalMs: 83.4, medianIntervalMs: 41.7, skippedFrames: 1 }), { frames: 25440, longIntervals: 57, skippedFrames: 3 })).toBe(
      "long=3 max=83ms med=41.7ms skip=1 fps=24.0/25.0 tot=57/3/25440",
    );
  });
});

describe("formatPainted", () => {
  it("adds Firefox's painted rate to both reports when it is known", () => {
    const w = win({ fps: 21, longIntervals: 3, maxIntervalMs: 81, medianIntervalMs: 40.4, skippedFrames: 2, paintedFps: 25 });
    expect(formatCadence(w, { frames: 100, longIntervals: 3, skippedFrames: 2 })).toBe(
      "long=3 max=81ms med=40.4ms skip=2 fps=21.0/25.0 paint=25.0 tot=3/2/100",
    );
    expect(formatRenderWindow({ ...w, fps: 10 })).toBe("fps=10.0/25.0 lag=0ms paint=25.0 jit=3/81ms sk=2");
  });
});

describe("createRenderStallWatch", () => {
  let clock = 0;
  const now = () => clock;

  function fakeVideo(withRvfc = true) {
    let frameCb: ((now?: number, meta?: { mediaTime?: number; expectedDisplayTime?: number }) => void) | null = null;
    const v = {
      paused: false,
      ended: false,
      requestVideoFrameCallback: withRvfc
        ? (cb: (now?: number, meta?: { mediaTime?: number; expectedDisplayTime?: number }) => void) => {
            frameCb = cb;
            return 1;
          }
        : undefined,
      cancelVideoFrameCallback: vi.fn(),
    } as unknown as HTMLVideoElement;
    return {
      v,
      present: () => frameCb?.(),
      // One frame put on screen at shownAt (ms), showing media time mediaTime (s).
      show: (shownAt: number, mediaTime: number) => frameCb?.(shownAt, { mediaTime, expectedDisplayTime: shownAt }),
    };
  }

  // Advances the injected clock and lets the watchdog's chained setTimeout run.
  async function advance(ms: number) {
    clock += ms;
    await vi.advanceTimersByTimeAsync(ms);
  }

  beforeEach(() => {
    clock = 0;
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it("reports a slowdown when the presented rate collapses, and names the main-thread lag", async () => {
    const onSlowdown = vi.fn();
    const { v, present } = fakeVideo();
    const watch = createRenderStallWatch(v, { onSlowdown, now });

    // First second at 20 frames: that becomes the baseline.
    for (let i = 0; i < 20; i++) present();
    await advance(1000);
    expect(onSlowdown).not.toHaveBeenCalled();

    // Second second: 4 frames, and the watchdog runs late because something
    // blocked the thread.
    for (let i = 0; i < 4; i++) present();
    clock += 400; // a block, observed as watchdog lateness
    await advance(1000);

    expect(onSlowdown).toHaveBeenCalledTimes(1);
    const w = onSlowdown.mock.calls[0][0];
    expect(w.fps).toBeLessThan(w.baselineFps * 0.66);
    expect(w.lagMaxMs).toBeGreaterThanOrEqual(400);
    watch.stop();
  });

  it("reports once per episode, not once per second", async () => {
    const onSlowdown = vi.fn();
    const { v, present } = fakeVideo();
    const watch = createRenderStallWatch(v, { onSlowdown, now });

    for (let i = 0; i < 20; i++) present();
    await advance(1000);
    for (let round = 0; round < 5; round++) {
      present();
      await advance(1000);
    }
    expect(onSlowdown).toHaveBeenCalledTimes(1);
    watch.stop();
  });

  it("says nothing at all on a paused player", async () => {
    const onSlowdown = vi.fn();
    const { v, present } = fakeVideo();
    (v as { paused: boolean }).paused = true;
    const watch = createRenderStallWatch(v, { onSlowdown, now });

    for (let i = 0; i < 20; i++) present();
    await advance(3000);
    expect(onSlowdown).not.toHaveBeenCalled();
    watch.stop();
  });

  // Older Safari has no requestVideoFrameCallback. Measuring nothing must not
  // turn into reporting everything.
  it("never reports without a frame callback to count with", async () => {
    const onSlowdown = vi.fn();
    const { v } = fakeVideo(false);
    const watch = createRenderStallWatch(v, { onSlowdown, now });
    await advance(4000);
    expect(onSlowdown).not.toHaveBeenCalled();
    expect(watch.latest()?.fps).toBe(0);
    watch.stop();
  });

  // One second of a 24 fps title; holds[i] extra frame durations before frame
  // i, skips[i] media frames jumped at frame i.
  function playSecond(show: (at: number, mt: number) => void, startMs: number, startFrame: number, holds: Record<number, number> = {}, skips: Record<number, number> = {}) {
    const dur = 1000 / 24;
    let at = startMs;
    let frame = startFrame;
    for (let i = 0; i < 24; i++) {
      at += dur * (1 + (holds[i] ?? 0));
      frame += 1 + (skips[i] ?? 0);
      show(at, frame / 24);
    }
    return { at, frame };
  }

  it("reports a picture that hitches at a full frame count", async () => {
    const onSlowdown = vi.fn();
    const onCadence = vi.fn();
    const { v, show } = fakeVideo();
    const watch = createRenderStallWatch(v, { onSlowdown, onCadence, now });

    let p = playSecond(show, 0, 0);
    await advance(1000);
    expect(onCadence).not.toHaveBeenCalled();

    // Three frames held for a second refresh, one media frame never shown.
    p = playSecond(show, p.at, p.frame, { 4: 1, 11: 1, 19: 1 }, { 7: 1 });
    await advance(1000);

    expect(onSlowdown).not.toHaveBeenCalled();
    expect(onCadence).toHaveBeenCalledTimes(1);
    const [w, totals] = onCadence.mock.calls[0];
    expect(w.longIntervals).toBe(3);
    expect(w.skippedFrames).toBe(1);
    expect(Math.round(w.maxIntervalMs)).toBe(83);
    expect(totals).toEqual({ frames: 48, longIntervals: 3, skippedFrames: 1 });
    expect(watch.totals()).toEqual(totals);
    watch.stop();
  });

  // Firefox's compositor count says whether a hitch reached the screen.
  it("reads Firefox's painted frames over the same window", async () => {
    const onCadence = vi.fn();
    const { v, show } = fakeVideo();
    const counted = v as HTMLVideoElement & { mozPaintedFrames: number };
    counted.mozPaintedFrames = 100;
    const watch = createRenderStallWatch(v, { onSlowdown: vi.fn(), onCadence, now });

    let p = playSecond(show, 0, 0);
    counted.mozPaintedFrames += 24;
    await advance(1000);
    p = playSecond(show, p.at, p.frame, { 4: 1, 11: 1, 19: 1 }, { 7: 1 });
    counted.mozPaintedFrames += 22;
    await advance(1000);

    expect(onCadence).toHaveBeenCalledTimes(1);
    expect(onCadence.mock.calls[0][0].paintedFps).toBe(22);
    expect(watch.latest()?.paintedFps).toBe(22);
    watch.stop();
  });

  // 24 fps on a 60 Hz panel alternates 33 and 50 ms by design.
  it("says nothing about the pulldown of a 60 Hz panel", async () => {
    const onCadence = vi.fn();
    const { v, show } = fakeVideo();
    const watch = createRenderStallWatch(v, { onSlowdown: vi.fn(), onCadence, now });
    let at = 0;
    for (let s = 0; s < 3; s++) {
      for (let i = 0; i < 24; i++) {
        at += i % 2 ? 50 : 33.3;
        show(at, (s * 24 + i) / 24);
      }
      await advance(1000);
    }
    expect(onCadence).not.toHaveBeenCalled();
    expect(watch.totals().longIntervals).toBe(0);
    watch.stop();
  });

  it("does not take a seek for skipped frames", async () => {
    const onCadence = vi.fn();
    const { v, show } = fakeVideo();
    const watch = createRenderStallWatch(v, { onSlowdown: vi.fn(), onCadence, now });
    const p = playSecond(show, 0, 0);
    await advance(1000);
    playSecond(show, p.at, p.frame + 24 * 600);
    await advance(1000);
    expect(watch.totals().skippedFrames).toBe(0);
    expect(onCadence).not.toHaveBeenCalled();
    watch.stop();
  });

  it("stops counting once stopped", async () => {
    const onSlowdown = vi.fn();
    const { v, present } = fakeVideo();
    const watch = createRenderStallWatch(v, { onSlowdown, now });
    for (let i = 0; i < 20; i++) present();
    await advance(1000);
    const seen = watch.latest();
    watch.stop();
    await advance(3000);
    expect(watch.latest()).toBe(seen);
    expect(onSlowdown).not.toHaveBeenCalled();
  });
});

describe("decideRenderCap", () => {
  const rungs = [360, 720, 1080, 2160];

  it("gives the rung up on the second report with a full buffer", () => {
    const s = { streak: 0, lastAt: 0 };
    expect(decideRenderCap(s, 1_000, 40, rungs, 2160)).toBeNull();
    expect(decideRenderCap(s, 31_000, 40, rungs, 2160)).toBe(1080);
    expect(s.streak).toBe(0);
  });

  it("blames the server, not the page, when the buffer is short", () => {
    const s = { streak: 1, lastAt: 1_000 };
    expect(decideRenderCap(s, 31_000, 2, rungs, 2160)).toBeNull();
    expect(s.streak).toBe(0);
  });

  it("does not chain two reports that are minutes apart", () => {
    const s = { streak: 0, lastAt: 0 };
    decideRenderCap(s, 1_000, 40, rungs, 2160);
    expect(decideRenderCap(s, 500_000, 40, rungs, 2160)).toBeNull();
  });

  it("judges a full buffer against the rung's own target: 6 s at 70 Mb/s", () => {
    const s = { streak: 1, lastAt: 1_000 };
    expect(decideRenderCap(s, 31_000, 5, rungs, 2160, 6)).toBe(1080);
    const short = { streak: 1, lastAt: 1_000 };
    expect(decideRenderCap(short, 31_000, 3, rungs, 2160, 6)).toBeNull();
    const wide = { streak: 1, lastAt: 1_000 };
    expect(decideRenderCap(wide, 31_000, 7, rungs, 2160, 30)).toBeNull();
  });

  it("has nowhere to go from the lowest rung", () => {
    const s = { streak: 1, lastAt: 1_000 };
    expect(decideRenderCap(s, 31_000, 40, rungs, 360)).toBeNull();
    expect(nextLowerRung(rungs, 2160)).toBe(1080);
    expect(nextLowerRung([1080], 1080)).toBeNull();
  });
});
