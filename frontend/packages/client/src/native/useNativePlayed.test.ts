import { describe, expect, it } from "vitest";
import type { PlayerAudioStream } from "@evey/api";
import { parseMasterAudioRenditions } from "../lib/masterRenditions";
import { nativePlayed } from "./useNativePlayed";

const streams: PlayerAudioStream[] = [
  { index: 1, trackIndex: 0, codec: "mp3", sourceTrackId: 2 },
  { index: 2, trackIndex: 1, codec: "ac3", sourceTrackId: 3 },
  { index: 3, trackIndex: 2, codec: "aac", sourceTrackId: 4 },
];
const URL_HLS = "https://h/api/v1/media/m/hls/master.m3u8?token=t&audioTrack=2";
const MASTER = `#EXTM3U
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aud-mp4a.40.34",NAME="Audio 1",X-MKVGO-TRACK="2",URI="od_audio1.m3u8"
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aud-mp4a.40.2",NAME="Stereo (AAC)",X-MKVGO-TRACK="4",URI="od_audio3.m3u8"
#EXT-X-STREAM-INF:BANDWIDTH=1,CODECS="avc1.640033,mp4a.40.34",AUDIO="aud-mp4a.40.34"
a.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=1,CODECS="avc1.640033,mp4a.40.2",AUDIO="aud-mp4a.40.2"
a.m3u8
`;
const master = { url: URL_HLS, renditions: parseMasterAudioRenditions(MASTER) };

// Same facts as the web player reports: the source track the loaded
// rendition carries, the codec the master declares for it, the picture.
describe("nativePlayed", () => {
  it("names the source track of the rendition the player plays", () => {
    const p = nativePlayed({ audioTrack: { id: "aud-mp4a.40.2:Stereo (AAC)", name: "Stereo (AAC)" }, videoTrack: { size: { height: 2160 } } }, URL_HLS, master, streams);
    expect(p).toEqual({ source: "hls", height: 2160, videoCodec: undefined, audioTrack: 2, audioCodec: "mp4a.40.2", audioDecoder: "native" });
  });
  it("names no track it cannot tell", () => {
    expect(nativePlayed({ audioTrack: { name: "Unknown" } }, URL_HLS, master, streams)?.audioTrack).toBe(-1);
    // The master read belongs to another stream (or none was read yet): the
    // track the URL asked for is not taken for the one playing.
    expect(nativePlayed({ audioTrack: { name: "Audio 1" } }, URL_HLS, { ...master, url: "other" }, streams)?.audioTrack).toBe(-1);
    expect(nativePlayed({ audioTrack: { name: "Audio 1" } }, URL_HLS, null, streams)?.audioTrack).toBe(-1);
  });
  it("reads the file's track by the number the file gives it", () => {
    const p = nativePlayed({ audioTrack: { id: "3" } }, "https://h/api/v1/media/m/stream?token=t", null, streams);
    expect(p).toEqual({ source: "file", audioTrack: 1, height: undefined, videoCodec: undefined });
  });
});
