import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { isRenderSlowdown, formatRenderWindow, createRenderStallWatch } from "./renderStall";

function win(over: Partial<Parameters<typeof isRenderSlowdown>[0]> = {}) {
  return { fps: 25, baselineFps: 25, lagMaxMs: 0, playing: true, visible: true, ...over };
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
});

describe("createRenderStallWatch", () => {
  let clock = 0;
  const now = () => clock;

  function fakeVideo(withRvfc = true) {
    let frameCb: (() => void) | null = null;
    const v = {
      paused: false,
      ended: false,
      requestVideoFrameCallback: withRvfc
        ? (cb: () => void) => {
            frameCb = cb;
            return 1;
          }
        : undefined,
      cancelVideoFrameCallback: vi.fn(),
    } as unknown as HTMLVideoElement;
    return { v, present: () => frameCb?.() };
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
