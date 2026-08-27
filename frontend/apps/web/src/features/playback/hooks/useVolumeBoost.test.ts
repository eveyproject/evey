import { renderHook, act } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { useVolumeBoost } from "./useVolumeBoost";

// Firefox regression harness: an AudioContext created inside the first boost
// gesture starts "suspended" there, and a fire-and-forget resume() leaves the
// gain ramp scheduled against a frozen clock - the boost never becomes
// audible while the UI happily shows 200% (préprod report, 2026-07-12).

class FakeGainParam {
  value = 1;
  setTargetAtTime = vi.fn((v: number) => { this.value = v; });
}

class FakeAudioContext {
  static instances: FakeAudioContext[] = [];
  state: AudioContextState = "suspended";
  currentTime = 0;
  resumeCalls = 0;
  resumeBehavior: "grant" | "deny" = "grant";
  onstatechange: (() => void) | null = null;
  gain = { gain: new FakeGainParam(), connect: vi.fn(), disconnect: vi.fn() };

  constructor() {
    FakeAudioContext.instances.push(this);
  }
  createMediaElementSource() {
    return { connect: vi.fn(), disconnect: vi.fn() };
  }
  createGain() {
    return this.gain;
  }
  createDynamicsCompressor() {
    return {
      threshold: { value: 0 }, knee: { value: 0 }, ratio: { value: 0 },
      attack: { value: 0 }, release: { value: 0 },
      connect: vi.fn(), disconnect: vi.fn(),
    };
  }
  resume() {
    this.resumeCalls++;
    if (this.resumeBehavior === "deny") {
      return Promise.reject(new DOMException("blocked", "NotAllowedError"));
    }
    // Real browsers resume asynchronously: the state flip and the clock start
    // land AFTER the calling gesture handler returned. A synchronous flip here
    // would mask the Firefox bug this suite exists for.
    return Promise.resolve().then(() => {
      this.state = "running";
      this.currentTime = 1.5; // the clock only moves once running
      this.onstatechange?.();
    });
  }
  close() { return Promise.resolve(); }
}

function setup() {
  const video = document.createElement("video");
  const videoRef = { current: video };
  const { result, unmount } = renderHook(() => useVolumeBoost(videoRef));
  return { setBoost: result.current, unmount, video };
}

const flush = () => act(async () => { await Promise.resolve(); await Promise.resolve(); });

describe("useVolumeBoost", () => {
  beforeEach(() => {
    FakeAudioContext.instances = [];
    vi.stubGlobal("AudioContext", FakeAudioContext as unknown as typeof AudioContext);
    Object.defineProperty(navigator, "userActivation", {
      configurable: true,
      value: { hasBeenActive: true },
    });
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("re-applies the requested gain once a suspended context resumes", async () => {
    const { setBoost } = setup();
    act(() => setBoost(2));
    await flush();

    const ctx = FakeAudioContext.instances[0];
    expect(ctx).toBeDefined();
    expect(ctx.resumeCalls).toBeGreaterThan(0);
    expect(ctx.state).toBe("running");
    // The ramp must be (re)scheduled against the RUNNING clock: a single call
    // made while suspended targets the frozen t=0 and, on Firefox, never
    // produces an audible boost.
    const calls = ctx.gain.gain.setTargetAtTime.mock.calls;
    const postResume = calls.filter(([gain, when]) => gain === 2 && when >= 1.5);
    expect(postResume.length).toBeGreaterThan(0);
  });

  it("keeps retrying resume on later boosts when the browser denied the first one", async () => {
    const { setBoost } = setup();
    act(() => setBoost(2));
    await flush();
    const ctx = FakeAudioContext.instances[0];
    ctx.resumeBehavior = "deny";
    ctx.state = "suspended";
    const denied = ctx.resumeCalls;

    act(() => setBoost(1.5));
    await flush();
    expect(ctx.resumeCalls).toBeGreaterThan(denied);
  });

  it("never builds a graph while the user stays at or under 100%", () => {
    const { setBoost } = setup();
    act(() => setBoost(1));
    expect(FakeAudioContext.instances).toHaveLength(0);
  });

  // Once createMediaElementSource captures the element, its audio ONLY exists
  // through this context. A context that drops to "suspended" while the picture
  // keeps playing is silent video, not just a frozen boost - the reported "le
  // son se coupe mais la vidéo continue". It must self-heal.
  it("resumes a context that goes suspended while the video is playing", async () => {
    const { setBoost, video } = setup();
    act(() => setBoost(2));
    await flush();
    const ctx = FakeAudioContext.instances[0];
    Object.defineProperty(video, "paused", { configurable: true, value: false });
    ctx.state = "suspended";
    const before = ctx.resumeCalls;
    act(() => { ctx.onstatechange?.(); }); // the browser fired the suspend
    expect(ctx.resumeCalls).toBeGreaterThan(before);
  });

  it("resumes on the video's own 'play' after a pause suspended the context", async () => {
    const { setBoost, video } = setup();
    act(() => setBoost(2));
    await flush();
    const ctx = FakeAudioContext.instances[0];
    ctx.state = "suspended"; // as if the pause/blur suspended it
    const before = ctx.resumeCalls;
    act(() => { video.dispatchEvent(new Event("play")); });
    expect(ctx.resumeCalls).toBeGreaterThan(before);
  });
});
