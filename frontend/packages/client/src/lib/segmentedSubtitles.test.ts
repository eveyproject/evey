import { describe, it, expect } from "vitest";
import { mergeCues, parseSubtitlePlaylist, segmentWindow, subtitleRenditionURI } from "./segmentedSubtitles";

// The od_master.m3u8 the server answered for a real MKV (a beta tester, 2026-10-05).
const MASTER = `#EXTM3U
#EXT-X-VERSION:7
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aud",NAME="French (France)",AUTOSELECT=YES,LANGUAGE="fre",CHANNELS="6",DEFAULT=YES,URI="od_audio1.m3u8?v=v0.38.0"
#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="subs",NAME="French (France) Forced",AUTOSELECT=NO,LANGUAGE="fre",DEFAULT=NO,FORCED=YES,X-EVEY-TRACK="0",URI="od_sub1.m3u8?v=v0.38.0"
#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="subs",NAME="French (France)",AUTOSELECT=NO,LANGUAGE="fre",X-EVEY-TRACK="1",URI="od_sub2.m3u8?v=v0.38.0"
#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="subs",NAME="SDH",AUTOSELECT=NO,LANGUAGE="eng",X-EVEY-TRACK="2",URI="od_sub3.m3u8?v=v0.38.0"
#EXT-X-STREAM-INF:BANDWIDTH=13260239,RESOLUTION=1920x1040,CODECS="avc1.640028,ac-3",AUDIO="aud",SUBTITLES="subs"
od_playlist.m3u8?v=v0.38.0
`;

const PLAYLIST = `#EXTM3U
#EXT-X-VERSION:7
#EXT-X-TARGETDURATION:7
#EXT-X-PLAYLIST-TYPE:VOD
#EXTINF:6.006,
od_sub2_00001.vtt?v=v0.38.0
#EXTINF:6.006,
od_sub2_00002.vtt?v=v0.38.0
#EXTINF:5.5,
od_sub2_00003.vtt?v=v0.38.0
#EXT-X-ENDLIST
`;

describe("subtitleRenditionURI", () => {
  it("finds the rendition by the picker's ordinal, not by its position in the master", () => {
    expect(subtitleRenditionURI(MASTER, 1)).toBe("od_sub2.m3u8?v=v0.38.0");
    expect(subtitleRenditionURI(MASTER, 0)).toBe("od_sub1.m3u8?v=v0.38.0");
  });

  it("answers null for a track the master does not carry (bitmap, external)", () => {
    expect(subtitleRenditionURI(MASTER, 3)).toBeNull();
    expect(subtitleRenditionURI("#EXTM3U\n", 0)).toBeNull();
  });
});

describe("parseSubtitlePlaylist", () => {
  it("places each segment on the film's timeline", () => {
    const segs = parseSubtitlePlaylist(PLAYLIST);
    expect(segs.map((s) => s.uri)).toEqual(["od_sub2_00001.vtt?v=v0.38.0", "od_sub2_00002.vtt?v=v0.38.0", "od_sub2_00003.vtt?v=v0.38.0"]);
    expect(segs[1].start).toBeCloseTo(6.006);
    expect(segs[2].end).toBeCloseTo(17.512);
  });
});

describe("segmentWindow", () => {
  const segs = parseSubtitlePlaylist(PLAYLIST);

  it("starts at the segment on screen and stops after the look-ahead", () => {
    expect(segmentWindow(segs, 7, 3)).toEqual([1]);
    expect(segmentWindow(segs, 7, 6)).toEqual([1, 2]);
  });

  it("keeps the segment just behind the playhead when asked", () => {
    expect(segmentWindow(segs, 7, 0, 2)).toEqual([0, 1]);
  });
});

describe("mergeCues", () => {
  it("drops the copy of a cue that spans two segments and keeps the list start-sorted", () => {
    const boundary = { start: 5.5, end: 7, text: "Il est formidable." };
    const merged = mergeCues([{ start: 1, end: 2, text: "a" }, boundary], [boundary, { start: 8, end: 9, text: "b" }]);
    expect(merged.map((c) => c.text)).toEqual(["a", "Il est formidable.", "b"]);
  });

  it("returns the same list when nothing is new, so React skips the render", () => {
    const list = [{ start: 1, end: 2, text: "a" }];
    expect(mergeCues(list, [{ start: 1, end: 2, text: "a" }])).toBe(list);
  });
});
