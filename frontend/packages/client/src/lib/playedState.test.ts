import { describe, expect, it } from "vitest";
import type { PlayerAudioStream } from "@evey/api";
import { masterTrackOrdinal, playedState, renditionTrackOrdinal, splitCodecs, trackRendition } from "./playedState";

// The BBB demo's shape: MP3 first, AAC second, AC-3 third, numbered by the file 2, 3, 4.
const streams: PlayerAudioStream[] = [
  { index: 1, trackIndex: 0, codec: "mp3", sourceTrackId: 2 },
  { index: 2, trackIndex: 1, codec: "aac", sourceTrackId: 3 },
  { index: 3, trackIndex: 2, codec: "ac3", sourceTrackId: 4 },
];

describe("renditionTrackOrdinal", () => {
  it("reads the packager's track number, not the rendition's place in the list", () => {
    // Renditions grouped by codec: the AAC group lists the AAC track alone,
    // at index 0 of hls.audioTracks.
    expect(renditionTrackOrdinal({ attrs: { "X-MKVGO-TRACK": "3" }, url: "od_audio1.m3u8" }, streams)).toBe(1);
    expect(renditionTrackOrdinal({ attrs: { "X-MKVGO-TRACK": "2" } }, streams)).toBe(0);
  });
  it("reads Evey's ffmpeg renditions by name", () => {
    expect(renditionTrackOrdinal({ url: "https://h/api/v1/media/m/hls/audio_2.m3u8?fp=x" }, streams)).toBe(2);
  });
  it("says it cannot tell rather than guess", () => {
    expect(renditionTrackOrdinal({ url: "other.m3u8" }, streams)).toBe(-1);
    expect(renditionTrackOrdinal({ attrs: { "X-MKVGO-TRACK": "99" } }, streams)).toBe(-1);
    expect(renditionTrackOrdinal(undefined, streams)).toBe(-1);
  });
});

describe("trackRendition", () => {
  it("finds the rendition of a track across every audio group", () => {
    const all = [
      { groupId: "aud-mp4a.40.34", attrs: { "X-MKVGO-TRACK": "2" } },
      { groupId: "aud-ac-3", attrs: { "X-MKVGO-TRACK": "3" } },
      { groupId: "aud-mp4a.40.2", attrs: { "X-MKVGO-TRACK": "4" } },
    ];
    expect(trackRendition(all, 0, streams)?.groupId).toBe("aud-mp4a.40.34");
    expect(trackRendition(all, 2, streams)?.groupId).toBe("aud-mp4a.40.2");
    expect(trackRendition(all, 5, streams)).toBeUndefined();
    expect(trackRendition(undefined, 0, streams)).toBeUndefined();
  });
});

describe("masterTrackOrdinal", () => {
  it("reads the track a muxed master was asked for", () => {
    expect(masterTrackOrdinal("/api/v1/media/m/hls/master.m3u8?token=t&audioTrack=1")).toBe(1);
    expect(masterTrackOrdinal("/api/v1/media/m/hls/master.m3u8?token=t")).toBe(-1);
  });
});

describe("splitCodecs", () => {
  it("splits a muxed buffer's codecs", () => {
    expect(splitCodecs("avc1.64001f,mp4a.40.2")).toEqual({ video: "avc1.64001f", audio: "mp4a.40.2" });
    expect(splitCodecs("mp4a.40.34")).toEqual({ audio: "mp4a.40.34" });
  });
});

describe("playedState", () => {
  it("reports the active rendition's track and the codec the buffer was set up for", () => {
    const p = playedState({
      hls: true, hasRenditions: true,
      activeRendition: { attrs: { "X-MKVGO-TRACK": "3" } },
      bufferCodecs: { video: "avc1.640028", audio: "mp4a.40.2" },
      height: 1080, audioStreams: streams,
    });
    expect(p).toEqual({ source: "hls", videoCodec: "avc1.640028", height: 1080, audioTrack: 1, audioCodec: "mp4a.40.2", audioDecoder: "native" });
  });
  it("reports the wasm decoder and the source track it decodes", () => {
    const p = playedState({ hls: true, wasmPath: true, wasmDecoding: true, wasmTrack: 2, audioStreams: streams });
    expect(p.audioDecoder).toBe("wasm");
    expect(p.audioTrack).toBe(2);
    expect(p.audioCodec).toBe("ac3");
  });
  it("names no codec on the relay's native leg", () => {
    const p = playedState({ hls: true, wasmPath: true, wasmDecoding: false, wasmTrack: 2, audioStreams: streams });
    expect(p.audioDecoder).toBe("native");
    expect(p.audioCodec).toBeUndefined();
  });
  it("reports the file and the track the element enabled", () => {
    expect(playedState({ hls: false, fileTrack: -1, audioStreams: streams })).toEqual({ source: "file", audioTrack: -1, height: undefined });
  });
});
