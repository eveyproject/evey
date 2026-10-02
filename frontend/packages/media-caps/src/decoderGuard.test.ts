import { describe, expect, it } from "vitest";
import { createDecoderWatchdog, formatDecoderCheck, parseDecoderCheck } from "./decoderGuard";

// 125 frames of E-AC-3 at 48 kHz = 4 s of audio, one watchdog window.
const at = (windows: number, msPerWindow: number, underruns = 0) => ({
  frames: 125 * windows,
  sampleRate: 48000,
  decodeMs: msPerWindow * windows,
  underruns,
});

describe("decoder watchdog", () => {
  it("gives up after two windows over 0.7 of real time, not after one", () => {
    const w = createDecoderWatchdog();
    expect(w(at(0, 3200))).toBeNull();
    expect(w(at(1, 3200))).toBeNull();
    expect(w(at(2, 3200))).toBe("rtf=0.80/ur=0,rtf=0.80/ur=0");
  });

  it("forgives a single slow window (a seek, a GC pause)", () => {
    const w = createDecoderWatchdog();
    w({ frames: 0, sampleRate: 48000, decodeMs: 0, underruns: 0 });
    expect(w({ frames: 125, sampleRate: 48000, decodeMs: 3200, underruns: 0 })).toBeNull();
    expect(w({ frames: 250, sampleRate: 48000, decodeMs: 3600, underruns: 0 })).toBeNull();
    expect(w({ frames: 375, sampleRate: 48000, decodeMs: 6800, underruns: 0 })).toBeNull();
  });

  // A network stall starves the output too, but leaves the decoder idle.
  it("counts underruns only while the decoder is visibly busy", () => {
    const idle = createDecoderWatchdog();
    idle(at(0, 400));
    expect(idle(at(1, 400, 5))).toBeNull();
    expect(idle(at(2, 400, 10))).toBeNull();

    const busy = createDecoderWatchdog();
    busy(at(0, 1600));
    busy(at(1, 1600, 3));
    expect(busy(at(2, 1600, 6))).toBe("rtf=0.40/ur=3,rtf=0.40/ur=3");
  });

  it("waits for a full window and starts over on a new decoder", () => {
    const w = createDecoderWatchdog();
    w(at(0, 3200));
    expect(w({ frames: 60, sampleRate: 48000, decodeMs: 1600, underruns: 0 })).toBeNull();
    expect(w(at(1, 3200))).toBeNull();
    // Counters went back: a track switch built a fresh decoder.
    expect(w({ frames: 10, sampleRate: 48000, decodeMs: 0, underruns: 0 })).toBeNull();
    expect(w({ frames: 135, sampleRate: 48000, decodeMs: 3200, underruns: 0 })).toBeNull();
  });
});

describe("decoder check", () => {
  it("parses the native map and refuses anything without a decoder list", () => {
    expect(parseDecoderCheck({ decoders: ["ac3", 3], blockedBy: "", bench: null })).toEqual({
      decoders: ["ac3"],
      blockedBy: "",
      bench: null,
      abnormalExit: null,
    });
    expect(parseDecoderCheck({ blockedBy: "bench" })).toBeNull();
    expect(parseDecoderCheck(null)).toBeNull();
  });

  it("formats a trusted decoder and a failed bench", () => {
    expect(
      formatDecoderCheck({ decoders: ["ac3", "eac3"], blockedBy: "", bench: { rtf: 0.0312, rtfCold: 0.05, rtfMax: 0.04, maxChannels: 8, wallMs: 140 }, abnormalExit: null }),
    ).toBe("declared=ac3,eac3 blocked=no rtf=0.031 cold=0.050 max=0.040 ch=8 wall=140ms");
    expect(formatDecoderCheck({ decoders: ["ac3", "eac3"], blockedBy: "", bench: { error: "no asset", slow: false }, abnormalExit: null })).toBe(
      "declared=ac3,eac3 blocked=no bench_error=no asset",
    );
  });
});
