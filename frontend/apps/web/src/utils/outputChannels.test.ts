import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import {
  clampOutputChannels,
  getMaxOutputChannels,
  resetOutputChannelsProbe,
} from "./outputChannels";

class FakeAudioContext {
  static instances: FakeAudioContext[] = [];
  static channels = 2;
  static throwOnConstruct = false;
  closed = false;
  destination: { maxChannelCount: number };
  constructor() {
    if (FakeAudioContext.throwOnConstruct) throw new Error("no audio device");
    this.destination = { maxChannelCount: FakeAudioContext.channels };
    FakeAudioContext.instances.push(this);
  }
  close() {
    this.closed = true;
    return Promise.resolve();
  }
}

function listeners(): Record<string, () => void> {
  const map: Record<string, () => void> = {};
  vi.stubGlobal("navigator", {
    ...navigator,
    mediaDevices: {
      addEventListener: (evt: string, cb: () => void) => {
        map[evt] = cb;
      },
    },
  });
  return map;
}

beforeEach(() => {
  FakeAudioContext.instances = [];
  FakeAudioContext.channels = 2;
  FakeAudioContext.throwOnConstruct = false;
  resetOutputChannelsProbe();
});

afterEach(() => {
  vi.unstubAllGlobals();
  resetOutputChannelsProbe();
});

describe("clampOutputChannels", () => {
  it("keeps a plausible reading and caps it at the 8 the server accepts", () => {
    expect(clampOutputChannels(2)).toBe(2);
    expect(clampOutputChannels(6)).toBe(6);
    // A pro/JACK output can advertise dozens of channels; the server rejects
    // anything above 8, so declaring it raw would silently lose the ceiling.
    expect(clampOutputChannels(32)).toBe(8);
  });

  it("returns undefined for everything that is not an answer", () => {
    expect(clampOutputChannels(undefined)).toBeUndefined();
    expect(clampOutputChannels(0)).toBeUndefined();
    expect(clampOutputChannels(-1)).toBeUndefined();
    expect(clampOutputChannels(NaN)).toBeUndefined();
    expect(clampOutputChannels("6")).toBeUndefined();
  });
});

describe("getMaxOutputChannels", () => {
  it("reports the OUTPUT device's channel count, not a decode capability", () => {
    FakeAudioContext.channels = 2;
    vi.stubGlobal("AudioContext", FakeAudioContext);
    expect(getMaxOutputChannels()).toBe(2);
  });

  it("keeps a real multichannel output multichannel", () => {
    FakeAudioContext.channels = 6;
    vi.stubGlobal("AudioContext", FakeAudioContext);
    expect(getMaxOutputChannels()).toBe(6);
  });

  it("closes the probe context instead of holding one open for the session", () => {
    vi.stubGlobal("AudioContext", FakeAudioContext);
    getMaxOutputChannels();
    expect(FakeAudioContext.instances).toHaveLength(1);
    expect(FakeAudioContext.instances[0].closed).toBe(true);
  });

  it("probes once and memoises", () => {
    vi.stubGlobal("AudioContext", FakeAudioContext);
    getMaxOutputChannels();
    getMaxOutputChannels();
    expect(FakeAudioContext.instances).toHaveLength(1);
  });

  it("answers undefined when the platform has no AudioContext at all", () => {
    vi.stubGlobal("AudioContext", undefined);
    vi.stubGlobal("webkitAudioContext", undefined);
    expect(getMaxOutputChannels()).toBeUndefined();
  });

  it("answers undefined when constructing the context throws", () => {
    FakeAudioContext.throwOnConstruct = true;
    vi.stubGlobal("AudioContext", FakeAudioContext);
    expect(getMaxOutputChannels()).toBeUndefined();
  });

  it("re-probes after a devicechange so plugging headphones is not ignored forever", () => {
    const events = listeners();
    FakeAudioContext.channels = 6;
    vi.stubGlobal("AudioContext", FakeAudioContext);
    expect(getMaxOutputChannels()).toBe(6);

    FakeAudioContext.channels = 2;
    expect(getMaxOutputChannels()).toBe(6); // still cached: no event yet
    events["devicechange"]?.();
    expect(getMaxOutputChannels()).toBe(2);
  });
});
