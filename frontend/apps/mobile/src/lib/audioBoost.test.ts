import { describe, it, expect, vi, beforeEach } from "vitest";

const setBoost = vi.fn(async () => true);
const release = vi.fn(async () => undefined);

vi.mock("react-native", () => ({
  Platform: { OS: "android" },
  NativeModules: {
    get AudioBoost() {
      return { setBoost, release };
    },
  },
}));

const { setAudioBoost, releaseAudioBoost, _resetAudioBoostForTest } = await import("./audioBoost");

// The bug this pins (2026-07-30, tablet): the volume slider and the vertical
// swipe both call setAudioBoost on EVERY move event (~60/s), and each call hit
// LoudnessEnhancer.setTargetGain + setEnabled on the global output mix. Dragging
// the volume down churned a live audio effect dozens of times a second and the
// sound broke up. The fix is here, not in the UI: the boost layer must coalesce.

beforeEach(() => {
  setBoost.mockClear();
  release.mockClear();
  _resetAudioBoostForTest();
});

describe("setAudioBoost", () => {
  it("collapses a drag into a single native call per meaningful step", async () => {
    // One continuous drag from 200% down to 100%, the way the swipe emits it.
    const drag = Array.from({ length: 60 }, (_, i) => 2 - i * (1 / 60));
    await Promise.all(drag.map((v) => setAudioBoost(v)));

    expect(setBoost.mock.calls.length).toBeLessThanOrEqual(12);
    // And whatever it coalesced, the LAST value the user asked for must win -
    // dropping the tail would leave the effect boosting after the user turned it down.
    const applied = setBoost.mock.calls.at(-1)?.[0] as number;
    expect(applied).toBeCloseTo(drag.at(-1)!, 1);
  });

  it("ignores an imperceptible change", async () => {
    await setAudioBoost(1.5);
    setBoost.mockClear();
    await setAudioBoost(1.51);
    expect(setBoost).not.toHaveBeenCalled();
  });

  it("still applies a real change", async () => {
    await setAudioBoost(1.5);
    setBoost.mockClear();
    await setAudioBoost(1.8);
    expect(setBoost).toHaveBeenCalledWith(1.8);
  });

  it("turns the effect off ONCE when leaving the boosted zone", async () => {
    await setAudioBoost(1.6);
    setBoost.mockClear();
    // Everything at or below 100% is "off" - repeating it must not re-touch the
    // effect on every pixel of the drag through the lower half of the bar.
    for (const v of [1, 0.9, 0.5, 0.2, 0]) await setAudioBoost(v);
    expect(setBoost.mock.calls.length).toBe(1);
    expect(setBoost.mock.calls[0][0]).toBeLessThanOrEqual(1);
  });

  it("never runs two native calls at once", async () => {
    let inFlight = 0;
    let overlapped = false;
    setBoost.mockImplementation(async () => {
      inFlight++;
      if (inFlight > 1) overlapped = true;
      await new Promise((r) => setTimeout(r, 1));
      inFlight--;
      return true;
    });
    await Promise.all([1.2, 1.4, 1.6, 1.8, 2].map((v) => setAudioBoost(v)));
    expect(overlapped).toBe(false);
  });

  it("re-arms after release so the next playback can boost again", async () => {
    await setAudioBoost(1.8);
    await releaseAudioBoost();
    setBoost.mockClear();
    await setAudioBoost(1.8);
    expect(setBoost).toHaveBeenCalledWith(1.8);
  });

  it("survives a native failure without wedging the layer", async () => {
    setBoost.mockRejectedValueOnce(new Error("no effect support"));
    await expect(setAudioBoost(1.5)).resolves.toBe(false);
    setBoost.mockClear();
    await setAudioBoost(1.9);
    expect(setBoost).toHaveBeenCalledWith(1.9);
  });

  // A drag back under 100% while a boosted value is still on its way: the
  // last event (0.95 -> off) was compared with what had LANDED (1.0) and
  // dropped, so the queue ended on 1.1 with the bar at 95%.
  it("ends on 'off' when the drag comes back under 100% mid-flight", async () => {
    await setAudioBoost(1);
    setBoost.mockImplementation(async () => { await new Promise((r) => setTimeout(r, 1)); return true; });
    await Promise.all([1.3, 1.1, 0.95].map((v) => setAudioBoost(v)));
    expect(setBoost.mock.calls.at(-1)?.[0]).toBe(1);
  });

  // A call still on its way when the player releases the boost must not
  // record itself as applied afterwards: the next 1.52 was skipped as "no
  // change" while nothing was boosting any more.
  it("forgets a call that lands after the release", async () => {
    setBoost.mockImplementation(async () => { await new Promise((r) => setTimeout(r, 1)); return true; });
    const late = setAudioBoost(1.5);
    await releaseAudioBoost();
    await late;
    setBoost.mockClear();
    await setAudioBoost(1.52);
    expect(setBoost).toHaveBeenCalledWith(1.52);
  });
});
