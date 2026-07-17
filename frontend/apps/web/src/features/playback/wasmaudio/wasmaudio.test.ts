import { describe, it, expect } from "vitest";
import { extractAudioURI, injectToken } from "./useWasmAudioPlayback";
import { channelSourceMap } from "./WasmAudioEngine";
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

describe("channelSourceMap", () => {
  it("remaps 5.1 from decoder L,C,R,Ls,Rs,LFE to Web Audio L,R,C,LFE,Ls,Rs", () => {
    // WA[0]=L<-0, WA[1]=R<-2, WA[2]=C<-1, WA[3]=LFE<-5, WA[4]=Ls<-3, WA[5]=Rs<-4
    expect(channelSourceMap(6)).toEqual([0, 2, 1, 5, 3, 4]);
  });
  it("passes stereo/mono and other layouts through unchanged", () => {
    expect(channelSourceMap(2)).toEqual([0, 1]);
    expect(channelSourceMap(1)).toEqual([0]);
    expect(channelSourceMap(8)).toEqual([0, 1, 2, 3, 4, 5, 6, 7]);
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
