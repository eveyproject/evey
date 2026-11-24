import { describe, expect, it } from "vitest";
import type { PlayerAudioStream } from "@evey/api";
import { nativeTrackForOrdinal, parseMasterAudioRenditions, renditionOfNativeTrack, renditionOrdinal } from "./masterRenditions";

// The demo's packager master as served (Big Buck Bunny): one audio
// group per codec, one rendition each, one variant per group.
const PACKAGER = `#EXTM3U
#EXT-X-VERSION:7
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aud-mp4a.40.34",NAME="Audio 1",AUTOSELECT=YES,CHANNELS="2",DEFAULT=YES,X-MKVGO-TRACK="2",URI="od_audio1.m3u8?v=x"
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aud-ac-3",NAME="Audio 2",AUTOSELECT=YES,CHANNELS="6",DEFAULT=YES,X-MKVGO-TRACK="3",URI="od_audio2.m3u8?v=x"
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aud-mp4a.40.2",NAME="Stereo (AAC)",AUTOSELECT=YES,CHANNELS="2",DEFAULT=YES,X-MKVGO-TRACK="4",URI="od_audio3.m3u8?v=x"
#EXT-X-STREAM-INF:BANDWIDTH=20601540,RESOLUTION=3840x2160,CODECS="avc1.640033,mp4a.40.34",AUDIO="aud-mp4a.40.34"
od_playlist.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=20760135,RESOLUTION=3840x2160,CODECS="avc1.640033,ac-3",AUDIO="aud-ac-3"
od_playlist.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=20636291,RESOLUTION=3840x2160,CODECS="avc1.640033,mp4a.40.2",AUDIO="aud-mp4a.40.2"
od_playlist.m3u8
`;

const FFMPEG = `#EXTM3U
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio",NAME="English",DEFAULT=YES,AUTOSELECT=YES,URI="audio_0.m3u8?fp=a"
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio",NAME="French",DEFAULT=NO,AUTOSELECT=YES,URI="audio_1.m3u8?fp=a"
#EXT-X-STREAM-INF:BANDWIDTH=4000000,CODECS="avc1.640028,mp4a.40.2",AUDIO="audio"
stream_0.m3u8
`;

const streams: PlayerAudioStream[] = [
  { index: 1, trackIndex: 0, codec: "mp3", sourceTrackId: 2 },
  { index: 2, trackIndex: 1, codec: "ac3", sourceTrackId: 3 },
  { index: 3, trackIndex: 2, codec: "aac", sourceTrackId: 4 },
];

describe("parseMasterAudioRenditions", () => {
  it("reads each rendition's group, name, source track and declared codec", () => {
    const r = parseMasterAudioRenditions(PACKAGER);
    expect(r).toEqual([
      { groupId: "aud-mp4a.40.34", name: "Audio 1", fileTrack: 2, codec: "mp4a.40.34" },
      { groupId: "aud-ac-3", name: "Audio 2", fileTrack: 3, codec: "ac-3" },
      { groupId: "aud-mp4a.40.2", name: "Stereo (AAC)", fileTrack: 4, codec: "mp4a.40.2" },
    ]);
    expect(r.map((x) => renditionOrdinal(x, streams))).toEqual([0, 1, 2]);
  });

  it("reads Evey's ffmpeg renditions by their name", () => {
    const r = parseMasterAudioRenditions(FFMPEG);
    expect(r.map((x) => x.audioOrdinal)).toEqual([0, 1]);
    // One group for both: the declared codec is the group's, not each one's.
    expect(r[1].codec).toBeUndefined();
  });
});

describe("native tracks", () => {
  const renditions = parseMasterAudioRenditions(PACKAGER);
  // Media3 names a rendition "GROUP-ID:NAME"; AVPlayer gives the NAME alone.
  const media3 = [{ id: "aud-mp4a.40.34:Audio 1", name: "Audio 1" }, { id: "aud-ac-3:Audio 2", name: "Audio 2" }, { id: "aud-mp4a.40.2:Stereo (AAC)", name: "Stereo (AAC)" }];
  const avplayer = [{ name: "Audio 1" }, { name: "Audio 2" }, { name: "Stereo (AAC)" }];

  it("tells which source track a native track plays", () => {
    expect(renditionOrdinal(renditionOfNativeTrack(renditions, media3[2]), streams)).toBe(2);
    expect(renditionOrdinal(renditionOfNativeTrack(renditions, avplayer[0]), streams)).toBe(0);
    expect(renditionOfNativeTrack(renditions, { name: "Unknown" })).toBeUndefined();
  });

  it("picks the native track of a source track in another audio group", () => {
    expect(nativeTrackForOrdinal(media3, renditions, 0, streams)).toBe(media3[0]);
    expect(nativeTrackForOrdinal(avplayer, renditions, 1, streams)).toBe(avplayer[1]);
    expect(nativeTrackForOrdinal(avplayer, renditions, 7, streams)).toBeUndefined();
  });

  it("names nothing when two renditions share a name and no id tells them apart", () => {
    const dup = parseMasterAudioRenditions(PACKAGER.replace('NAME="Audio 2"', 'NAME="Audio 1"'));
    expect(renditionOfNativeTrack(dup, { name: "Audio 1" })).toBeUndefined();
    expect(renditionOfNativeTrack(dup, { id: "aud-ac-3:Audio 1", name: "Audio 1" })?.fileTrack).toBe(3);
  });
});
