import { describe, it, expect } from "vitest";
import { extractAudioURI, injectToken } from "./useWasmAudioPlayback";
import { outputMix, FOLD_GAIN } from "./channelMix";
import { isWasmDecodableAudioCodec } from "@/utils/codec";

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
