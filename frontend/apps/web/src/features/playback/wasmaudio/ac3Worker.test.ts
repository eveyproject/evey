import { describe, it, expect } from "vitest";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { outputMix } from "./channelMix";
import { fillAudioBuffer, workerMixTable, type PcmDecoded } from "./WasmAudioEngine";

// The worker is a plain static file (no bundler, classic worker). Loading it
// with a fake `self` runs its top level - which defines the handlers and
// exposes mixPlanar - without touching the wasm runtime (that only happens on
// an init message we never send).
function loadWorkerHooks(): { mixPlanar: (planar: Float32Array[], frames: number, mix: Array<Array<[number, number]>>) => Float32Array[] } {
  const code = readFileSync(resolve(__dirname, "../../../../public/wasm/ac3Worker.js"), "utf8");
  const self: Record<string, unknown> = { postMessage: () => {} };
  new Function("self", code)(self);
  return self.eveyWorkerTestHooks as ReturnType<typeof loadWorkerHooks>;
}

// Three code paths can turn a decoded segment into output channels: the
// worker's mixPlanar (new), the main thread's planar fold, and the main
// thread's interleaved fold (a cached old worker). A viewer must not be able
// to hear which one their browser took, so all three are held to the SAME
// samples, bit for bit, on the layout where it matters most - 7.1, the only
// one with arithmetic in it.
class CaptureBuffer {
  private readonly data: Float32Array[];
  constructor(numberOfChannels: number, length: number) {
    this.data = Array.from({ length: numberOfChannels }, () => new Float32Array(length));
  }
  copyToChannel(source: Float32Array, channel: number): void {
    this.data[channel].set(source);
  }
  getChannelData(channel: number): Float32Array {
    return this.data[channel];
  }
}

function sourceSignal(channels: number, frames: number): Float32Array[] {
  // Deterministic but not trivial: fractions that do not round cleanly in
  // float32, so a wrong gain or a swapped tap cannot hide behind integers.
  return Array.from({ length: channels }, (_, c) =>
    Float32Array.from({ length: frames }, (_, f) => Math.sin(c * 1.7 + f * 0.31) * (0.5 + c / 10)),
  );
}

function interleave(planar: Float32Array[], frames: number): Float32Array {
  const ch = planar.length;
  const out = new Float32Array(frames * ch);
  for (let f = 0; f < frames; f++) for (let c = 0; c < ch; c++) out[f * ch + c] = planar[c][f];
  return out;
}

describe("ac3Worker mixPlanar", () => {
  const FRAMES = 64;
  const { mixPlanar } = loadWorkerHooks();

  for (const channels of [1, 2, 6, 8]) {
    it(`produces the same samples as both main-thread paths for ${channels} channels`, () => {
      const mix = outputMix(channels);
      const src = sourceSignal(channels, FRAMES);

      const fromWorker = mixPlanar(src, FRAMES, mix);

      const viaPlanar = new CaptureBuffer(mix.length, FRAMES);
      fillAudioBuffer(
        viaPlanar as unknown as AudioBuffer,
        { kind: "pcm", channels, sampleRate: 48000, frames: FRAMES, planar: src as Float32Array<ArrayBuffer>[] } satisfies PcmDecoded,
        mix,
      );

      const viaInterleaved = new CaptureBuffer(mix.length, FRAMES);
      fillAudioBuffer(
        viaInterleaved as unknown as AudioBuffer,
        { kind: "pcm", channels, sampleRate: 48000, frames: FRAMES, pcm: interleave(src, FRAMES) },
        mix,
      );

      expect(fromWorker).toHaveLength(mix.length);
      for (let co = 0; co < mix.length; co++) {
        expect(Array.from(fromWorker[co])).toEqual(Array.from(viaPlanar.getChannelData(co)));
        expect(Array.from(fromWorker[co])).toEqual(Array.from(viaInterleaved.getChannelData(co)));
      }
    });
  }

  it("reuses the source array for a pure permutation instead of copying it", () => {
    const src = sourceSignal(6, FRAMES);
    const out = mixPlanar(src, FRAMES, outputMix(6));
    // 5.1 is a permutation: every output IS one of the inputs.
    for (const o of out) expect(src).toContain(o);
  });

  it("allocates fresh arrays only for the folded 7.1 surrounds", () => {
    const src = sourceSignal(8, FRAMES);
    const out = mixPlanar(src, FRAMES, outputMix(8));
    expect(out.slice(0, 4).every((o) => src.includes(o))).toBe(true);
    expect(src.includes(out[4])).toBe(false);
    expect(src.includes(out[5])).toBe(false);
  });
});

describe("workerMixTable", () => {
  it("hands the worker exactly outputMix, for every layout the decoder can produce", () => {
    const table = workerMixTable();
    for (let ch = 1; ch <= 8; ch++) expect(table[ch]).toEqual(outputMix(ch));
  });
});
