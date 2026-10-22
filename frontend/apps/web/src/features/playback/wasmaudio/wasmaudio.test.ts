import { describe, it, expect, vi } from "vitest";
import { extractAudioURI, injectToken } from "./useWasmAudioPlayback";
import { outputMix, FOLD_GAIN } from "./channelMix";
import { isWasmDecodableAudioCodec } from "@/utils/codec";
import { fillAudioBuffer, mayResumeAudioContext, medianOf, type PcmDecoded } from "./WasmAudioEngine";

describe("extractAudioURI", () => {
  it("pulls the quoted rendition URI out of the master", () => {
    const master = [
      "#EXTM3U",
      "#EXT-X-VERSION:6",
      "#EXT-X-INDEPENDENT-SEGMENTS",
      '#EVEY-AUDIO-URI:"od_audio1.m3u8?token=abc&v=0.25.0"',
      "#EXT-X-STREAM-INF:BANDWIDTH=8000000,CODECS=\"avc1.640028\"",
      "od_playlist.m3u8?token=abc",
    ].join("\n");
    expect(extractAudioURI(master)).toBe("od_audio1.m3u8?token=abc&v=0.25.0");
  });

  it("handles the ffmpeg rendition URI", () => {
    const master = '#EXTM3U\n#EVEY-AUDIO-URI:"audio_0.m3u8?wasmAudio=1&token=x"\nstream_0.m3u8';
    expect(extractAudioURI(master)).toBe("audio_0.m3u8?wasmAudio=1&token=x");
  });

  it("returns null when the tag is absent (server did not take the wasm path)", () => {
    const master = "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1,CODECS=\"avc1,mp4a.40.2\"\nstream_0.m3u8";
    expect(extractAudioURI(master)).toBeNull();
  });
});

describe("injectToken", () => {
  it("overrides an existing token param with the fresh one", () => {
    expect(injectToken("/api/v1/media/m/hls/od_audio1.m3u8?token=old&v=1", "new")).toBe(
      "/api/v1/media/m/hls/od_audio1.m3u8?token=new&v=1",
    );
  });
  it("adds a token when none is present", () => {
    expect(injectToken("/api/v1/media/m/hls/seg1.m4s?v=1", "tok")).toContain("token=tok");
  });
  it("is a no-op with an empty token", () => {
    expect(injectToken("/x?token=keep", "")).toBe("/x?token=keep");
  });
});

describe("outputMix", () => {
  it("remaps 5.1 decoder L,C,R,Ls,Rs,LFE to Web Audio L,R,C,LFE,SL,SR", () => {
    expect(outputMix(6)).toEqual([[[0, 1]], [[2, 1]], [[1, 1]], [[5, 1]], [[3, 1]], [[4, 1]]]);
  });
  it("folds 7.1 decoder L,R,C,LFE,BL,BR,SL,SR to 5.1 with -3 dB on the folded taps", () => {
    const m = outputMix(8);
    expect(m).toHaveLength(6); // folded to 5.1 so the browser can down-mix (keeps centre on stereo)
    expect(m[0]).toEqual([[0, 1]]); // L
    expect(m[1]).toEqual([[1, 1]]); // R
    expect(m[2]).toEqual([[2, 1]]); // C
    expect(m[3]).toEqual([[3, 1]]); // LFE
    // Summing two channels at unity gain is +6 dB of headroom given away
    // before the browser's own un-normalised stereo fold even runs.
    expect(m[4]).toEqual([[6, FOLD_GAIN], [4, FOLD_GAIN]]); // SL <- (SL + BL) * .7071
    expect(m[5]).toEqual([[7, FOLD_GAIN], [5, FOLD_GAIN]]); // SR <- (SR + BR) * .7071
    expect(FOLD_GAIN).toBeCloseTo(0.7071, 4);
  });

  it("keeps the 7.1 worst case below what unity-gain folding produced", () => {
    // Peak of one Web Audio output channel when every decoder channel it taps
    // is simultaneously at full scale.
    const peak = (taps: Array<[number, number]>) => taps.reduce((a, [, g]) => a + g, 0);
    const m = outputMix(8);
    expect(Math.max(...m.map(peak))).toBeCloseTo(Math.SQRT2, 4); // was 2.0
    // What the stereo listener actually gets: the browser's "speakers" matrix
    // (L=1, C=.7071, SL=.7071) applied on top of our 5.1 buffer.
    const stereoPeak = peak(m[0]) + 0.7071 * peak(m[2]) + 0.7071 * peak(m[4]);
    expect(stereoPeak).toBeLessThan(3.12); // the unity-gain fold's worst case
    expect(stereoPeak).toBeCloseTo(2.707, 2);
  });
  it("passes stereo/mono and other layouts through unchanged (identity taps)", () => {
    expect(outputMix(2)).toEqual([[[0, 1]], [[1, 1]]]);
    expect(outputMix(1)).toEqual([[[0, 1]]]);
  });
});

describe("isWasmDecodableAudioCodec", () => {
  it("accepts AC-3 / E-AC-3 in every spelling", () => {
    for (const c of ["ac3", "ac-3", "eac3", "e-ac-3", "ec-3", "EAC3"]) {
      expect(isWasmDecodableAudioCodec(c)).toBe(true);
    }
  });
  it("rejects everything else", () => {
    for (const c of ["aac", "flac", "opus", "dts", "truehd", "", undefined]) {
      expect(isWasmDecodableAudioCodec(c)).toBe(false);
    }
  });
});

// FakeAudioBuffer stands in for the real Web Audio AudioBuffer (unavailable in
// jsdom): it records every copyToChannel call so the tests can assert
// fillAudioBuffer takes the native-copy path, not a JS loop, for a plain
// permutation tap.
class FakeAudioBuffer {
  readonly copyToChannelCalls: Array<{ source: Float32Array; channel: number }> = [];
  private readonly data: Float32Array[];
  constructor(numberOfChannels: number, length: number) {
    this.data = Array.from({ length: numberOfChannels }, () => new Float32Array(length));
  }
  copyToChannel(source: Float32Array, channelNumber: number): void {
    this.copyToChannelCalls.push({ source, channel: channelNumber });
    this.data[channelNumber].set(source);
  }
  getChannelData(channel: number): Float32Array {
    return this.data[channel];
  }
}

describe("fillAudioBuffer", () => {
  const FRAMES = 4;

  // The worker can now hand back OUTPUT channels with the fold already
  // applied. Then even 7.1 - the one layout with real arithmetic - costs the
  // main thread nothing but native copies.
  it("copies a mixed 7.1 reply channel for channel and never touches a sample", () => {
    const mix = outputMix(8);
    const planar = Array.from({ length: mix.length }, (_, co) =>
      Float32Array.from({ length: FRAMES }, (_, f) => co * 10 + f),
    );
    const d: PcmDecoded = { kind: "pcm", channels: 8, sampleRate: 48000, frames: FRAMES, planar, mixed: true };
    const target = new FakeAudioBuffer(mix.length, FRAMES);
    const getChannelData = vi.spyOn(target, "getChannelData");

    fillAudioBuffer(target, d, mix);

    expect(target.copyToChannelCalls.map((c) => c.channel)).toEqual([0, 1, 2, 3, 4, 5]);
    expect(getChannelData).not.toHaveBeenCalled();
    for (let co = 0; co < mix.length; co++) {
      expect(Array.from(target.getChannelData(co))).toEqual(Array.from(planar[co]));
    }
  });

  // A source channel's samples are all == its own index * 10 + frame, so the
  // schedule/mix mapping can be read straight off the output values.
  function planarSource(channels: number): Float32Array[] {
    return Array.from({ length: channels }, (_, c) => {
      const a = new Float32Array(FRAMES);
      for (let f = 0; f < FRAMES; f++) a[f] = c * 10 + f;
      return a;
    });
  }

  function interleave(planar: Float32Array[]): Float32Array {
    const channels = planar.length;
    const out = new Float32Array(channels * FRAMES);
    for (let f = 0; f < FRAMES; f++) {
      for (let c = 0; c < channels; c++) out[f * channels + c] = planar[c][f];
    }
    return out;
  }

  it("lands each 5.1 source channel on the output channel outputMix dictates (planar)", () => {
    const planar = planarSource(6);
    const decoded: PcmDecoded = { kind: "pcm", channels: 6, sampleRate: 48000, frames: FRAMES, planar };
    const mix = outputMix(6);
    const target = new FakeAudioBuffer(mix.length, FRAMES);

    fillAudioBuffer(target as unknown as AudioBuffer, decoded, mix);

    // mix(6) = L<-0, R<-2, C<-1, LFE<-5, SL<-3, SR<-4 - a pure permutation.
    expect(Array.from(target.getChannelData(0))).toEqual(Array.from(planar[0]));
    expect(Array.from(target.getChannelData(1))).toEqual(Array.from(planar[2]));
    expect(Array.from(target.getChannelData(2))).toEqual(Array.from(planar[1]));
    expect(Array.from(target.getChannelData(3))).toEqual(Array.from(planar[5]));
    expect(Array.from(target.getChannelData(4))).toEqual(Array.from(planar[3]));
    expect(Array.from(target.getChannelData(5))).toEqual(Array.from(planar[4]));
    // Every output channel is a single unity tap: copyToChannel, not a loop.
    expect(target.copyToChannelCalls).toHaveLength(6);
  });

  it("produces the identical result from the legacy interleaved shape (compat proof)", () => {
    const planar = planarSource(6);
    const mix = outputMix(6);

    const planarTarget = new FakeAudioBuffer(mix.length, FRAMES);
    fillAudioBuffer(
      planarTarget as unknown as AudioBuffer,
      { kind: "pcm", channels: 6, sampleRate: 48000, frames: FRAMES, planar },
      mix,
    );

    const interleavedTarget = new FakeAudioBuffer(mix.length, FRAMES);
    fillAudioBuffer(
      interleavedTarget as unknown as AudioBuffer,
      { kind: "pcm", channels: 6, sampleRate: 48000, frames: FRAMES, pcm: interleave(planar) },
      mix,
    );

    for (let co = 0; co < mix.length; co++) {
      expect(Array.from(interleavedTarget.getChannelData(co))).toEqual(
        Array.from(planarTarget.getChannelData(co)),
      );
    }
    // The interleaved path never sees copyToChannel - it is the fallback loop.
    expect(interleavedTarget.copyToChannelCalls).toHaveLength(0);
  });

  it("still folds 7.1 with both taps and their gain, reading from the planar arrays", () => {
    const planar = planarSource(8);
    const mix = outputMix(8);
    const target = new FakeAudioBuffer(mix.length, FRAMES);

    fillAudioBuffer(
      target as unknown as AudioBuffer,
      { kind: "pcm", channels: 8, sampleRate: 48000, frames: FRAMES, planar },
      mix,
    );

    // SL (output 4) <- (source 6 + source 4) * FOLD_GAIN
    // Round-trip through Float32Array so the expected values carry the same
    // single-precision rounding as fillAudioBuffer's output.
    const expectedSL = Array.from(
      new Float32Array(FRAMES).map((_, f) => (planar[6][f] + planar[4][f]) * FOLD_GAIN),
    );
    const expectedSR = Array.from(
      new Float32Array(FRAMES).map((_, f) => (planar[7][f] + planar[5][f]) * FOLD_GAIN),
    );
    expect(Array.from(target.getChannelData(4))).toEqual(expectedSL);
    expect(Array.from(target.getChannelData(5))).toEqual(expectedSR);
    // The two folded outputs used the loop, not a native copy.
    expect(target.copyToChannelCalls.map((c) => c.channel)).not.toContain(4);
    expect(target.copyToChannelCalls.map((c) => c.channel)).not.toContain(5);
    // The four unity-tap outputs (L,R,C,LFE) still took the native-copy path.
    expect(target.copyToChannelCalls).toHaveLength(4);
  });
});

// Regression, 2026-09-19: browsing the settings panel on a PAUSED film played
// a tenth of a second of sound at every click. The engine's gesture unlock
// listens on the window for its whole life and re-armed + resumed the context
// on each pointerdown; the context suspended by the pause still held its
// scheduled sources, so they became audible until the next 100ms tick
// suspended it again.
describe("mayResumeAudioContext", () => {
  it("refuses while the picture is paused, however the wake was triggered", () => {
    expect(mayResumeAudioContext({ state: "suspended", armed: true, videoPaused: true })).toBe(false);
  });

  it("allows the wake once the picture runs again", () => {
    expect(mayResumeAudioContext({ state: "suspended", armed: true, videoPaused: false })).toBe(true);
  });

  it("stays disarmed after a refused resume, and ignores a running context", () => {
    expect(mayResumeAudioContext({ state: "suspended", armed: false, videoPaused: false })).toBe(false);
    expect(mayResumeAudioContext({ state: "running", armed: true, videoPaused: false })).toBe(false);
  });
});

describe("medianOf", () => {
  it("takes the middle sample, so a stray tick does not move the drift", () => {
    expect(medianOf([0.01, 0.012, 0.3, 0.011, 0.009])).toBe(0.011);
    expect(medianOf([1, 3])).toBe(2);
    expect(medianOf([])).toBeNaN();
  });
});
