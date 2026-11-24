import { describe, expect, it } from "vitest";
import { codecName, describeSession } from "./servedReality";

describe("codecName", () => {
  it("speaks one vocabulary for the server's names and the browser's RFC 6381 strings", () => {
    expect(codecName("mp4a.40.2")).toBe("aac");
    expect(codecName("mp4a.40.34")).toBe("mp3");
    expect(codecName("mp3")).toBe("mp3");
    expect(codecName("avc1.640028")).toBe("h264");
    expect(codecName("hvc1.2.4.L150")).toBe("hevc");
    expect(codecName("ac-3")).toBe("ac3");
    expect(codecName("ec-3")).toBe("eac3");
    expect(codecName("")).toBe("");
  });
});

describe("describeSession", () => {
  it("names nothing before any byte was served", () => {
    const r = describeSession({});
    expect(r.method).toBe("");
    expect(r.video).toBeUndefined();
    expect(r.audio).toBeUndefined();
  });

  it("is a direct play when the file itself went out", () => {
    const r = describeSession({
      servedVideo: { engine: "file", copy: true, track: -1, codec: "h264", height: 1080 },
      servedAudio: { engine: "file", copy: true, track: -1 },
      played: { source: "file", audioTrack: 1 },
    });
    expect(r.method).toBe("direct_play");
    expect(r.audio?.track).toBe(-1);
    expect(r.trackMismatch).toBeUndefined();
  });

  it("reads an absent track as track 0 (proto3 JSON leaves zeros out)", () => {
    const r = describeSession({ servedAudio: { engine: "mkvgo", copy: true, codec: "aac" } });
    expect(r.audio?.track).toBe(0);
  });

  it("is a transcode when a leg was encoded, and names its source codec", () => {
    const r = describeSession({
      servedVideo: { engine: "mkvgo", copy: true, track: -1, codec: "h264" },
      servedAudio: { engine: "ffmpeg", track: 1, codec: "aac", sourceCodec: "mp3" },
    });
    expect(r.method).toBe("transcode");
    expect(r.audio?.sourceCodec).toBe("mp3");
  });

  // The case measured on the demo: the player said it switched to the MP3 track
  // while the server kept sending the AAC one. Both facts show, side by side.
  it("says so when the player plays another track than the one sent", () => {
    const r = describeSession({
      servedAudio: { engine: "mkvgo", copy: true, track: 2, codec: "aac" },
      played: { source: "hls", audioTrack: 1, audioCodec: "mp4a.40.2", audioDecoder: "native" },
    });
    expect(r.trackMismatch).toEqual({ played: 1, served: 2 });
    expect(r.codecMismatch).toBeUndefined();
  });

  it("says so when the decoder was set up for another codec than the one sent", () => {
    const r = describeSession({
      servedAudio: { engine: "ffmpeg", copy: true, track: 0, codec: "ac3" },
      played: { source: "hls", audioTrack: 0, audioCodec: "mp4a.40.2", audioDecoder: "native" },
    });
    expect(r.codecMismatch).toEqual({ played: "aac", served: "ac3" });
  });

  it("agrees with a wasm decoder fed the copied AC-3", () => {
    const r = describeSession({
      servedAudio: { engine: "ffmpeg", copy: true, track: 0, codec: "ac3" },
      played: { source: "hls", audioCodec: "ac-3", audioDecoder: "wasm" },
    });
    expect(r.method).toBe("direct_stream");
    expect(r.played?.decoder).toBe("wasm");
    expect(r.codecMismatch).toBeUndefined();
    expect(r.trackMismatch).toBeUndefined();
  });
});
