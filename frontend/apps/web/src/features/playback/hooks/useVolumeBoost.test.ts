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
  comp = {
    threshold: { value: 0 }, knee: { value: 0 }, ratio: { value: 0 },
    attack: { value: 0 }, release: { value: 0 },
    connect: vi.fn(), disconnect: vi.fn(),
  };
  destination = { id: "destination" };
  createDynamicsCompressor() {
    return this.comp;
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

// The limiter is there to round off what the AMPLIFICATION overshoots. Leaving
// it wired at unity gain compresses ordinary programme material at 12:1 with a
// 250 ms release - heard as "the levels jump around and it distorts", and it
// survived every later playback because createMediaElementSource captures the
// element for good (preprod report, 2026-09-02: distortion vs Jellyfin on a
// plain stereo AAC file the server was copying byte for byte).
describe("useVolumeBoost - limiter routing", () => {
  // Same fixture as the suite above: a sibling describe does not inherit its
  // beforeEach, so without this the hook builds a REAL AudioContext and the
  // assertions read a stale instance left over from the previous block.
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

  it("wires the limiter in only while boosting, and takes it back out at 100%", async () => {
    const { setBoost } = setup();

    act(() => { setBoost(1.5); });
    await flush();
    const ctx = FakeAudioContext.instances[0];

    // Boosting wires gain -> compressor -> destination.
    expect(ctx.gain.connect).toHaveBeenCalledWith(ctx.comp);
    expect(ctx.comp.connect).toHaveBeenCalledWith(ctx.destination);

    ctx.gain.connect.mockClear();
    ctx.comp.connect.mockClear();
    ctx.comp.disconnect.mockClear();

    // Staying inside the boosted zone must not re-wire a live graph - that
    // churn is what broke the mobile boost once.
    act(() => { setBoost(1.7); });
    await flush();
    expect(ctx.gain.connect).not.toHaveBeenCalled();

    // Back to 100%: straight gain -> destination, compressor unwired.
    act(() => { setBoost(1); });
    await flush();
    expect(ctx.comp.disconnect).toHaveBeenCalled();
    expect(ctx.gain.connect).toHaveBeenCalledWith(ctx.destination);
    expect(ctx.comp.connect).not.toHaveBeenCalled();
  });
});
