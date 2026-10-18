import { describe, it, expect } from "vitest";
import { nominalQualityTier, parseHlsMasterHeights, parseHlsMasterResolutions, qualityRungsFromHeights, resolutionTier } from "./hlsRungs";

const MASTER = `#EXTM3U
#EXT-X-VERSION:6
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aud",NAME="French",DEFAULT=YES,URI="audio/fr.m3u8"
#EXT-X-STREAM-INF:BANDWIDTH=8000000,RESOLUTION=1920x798,CODECS="avc1.640028,mp4a.40.2",AUDIO="aud"
v0/index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=3000000,RESOLUTION=1280x532,CODECS="avc1.4d401f,mp4a.40.2",AUDIO="aud"
v1/index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=800000,CODECS="mp4a.40.2",AUDIO="aud"
audio-only/index.m3u8
`;

describe("nominalQualityTier", () => {
  // A widescreen master is scaled to a rung WIDTH, so its height is never the
  // round number the rung is named after. 798p on screen reads as a bug.
  it("rounds an encoded height to the tier a viewer recognises", () => {
    expect(nominalQualityTier(798)).toBe(720);
    expect(nominalQualityTier(532)).toBe(480);
    expect(nominalQualityTier(1040)).toBe(1080);
  });

  it("leaves an exact height alone", () => {
    expect(nominalQualityTier(1080)).toBe(1080);
    expect(nominalQualityTier(2160)).toBe(2160);
  });

  it("has no tier for nothing", () => {
    expect(nominalQualityTier(0)).toBe(0);
    expect(nominalQualityTier(-5)).toBe(0);
  });
});

// The quality menu and the title page name the same frame the same way: a
// copied 1920x818 scope film read "1080p" on the page and "Auto (720p)" in the
// player (demo, 2026-10-01).
describe("a rung's label agrees with the title page", () => {
  it("names a scope rung by its width once the width is known", () => {
    expect(nominalQualityTier(818, 1920)).toBe(1080);
    expect(nominalQualityTier(798, 1920)).toBe(resolutionTier(1920, 798));
    expect(nominalQualityTier(532, 1280)).toBe(720);
    expect(nominalQualityTier(720, 1280)).toBe(720);
    expect(nominalQualityTier(1080, 1920)).toBe(1080);
  });

  it("labels the menu from the master's widths", () => {
    expect(parseHlsMasterResolutions(MASTER)).toEqual([{ width: 1920, height: 798 }, { width: 1280, height: 532 }]);
    expect(qualityRungsFromHeights(parseHlsMasterResolutions(MASTER))).toEqual([
      { value: 798, tier: 1080, label: "1080p" },
      { value: 532, tier: 720, label: "720p" },
    ]);
  });
});

describe("parseHlsMasterHeights", () => {
  it("reads the height of every video variant, in playlist order", () => {
    expect(parseHlsMasterHeights(MASTER)).toEqual([798, 532]);
  });

  // The audio rendition has no RESOLUTION and is not a quality choice: an
  // "audio only" entry in a quality menu is a trap, not an option.
  it("ignores a variant with no resolution", () => {
    expect(parseHlsMasterHeights(MASTER)).not.toContain(0);
  });

  // A caller that fetched the wrong URL - a media playlist, an error page, an
  // empty body - must fall back to its static ladder, not show an empty menu.
  it("finds nothing in something that is not a master playlist", () => {
    expect(parseHlsMasterHeights("#EXTM3U\n#EXTINF:6.0,\nseg0.ts\n")).toEqual([]);
    expect(parseHlsMasterHeights("<html>404</html>")).toEqual([]);
    expect(parseHlsMasterHeights("")).toEqual([]);
  });

  it("survives CRLF and lowercase attributes", () => {
    const crlf = "#EXTM3U\r\n#EXT-X-STREAM-INF:BANDWIDTH=1,resolution=1280x720\r\nv/i.m3u8\r\n";
    expect(parseHlsMasterHeights(crlf)).toEqual([720]);
  });
});

describe("qualityRungsFromHeights", () => {
  it("offers the tallest first, labelled by tier and valued by exact height", () => {
    expect(qualityRungsFromHeights([532, 798])).toEqual([
      { value: 798, tier: 720, label: "720p" },
      { value: 532, tier: 480, label: "480p" },
    ]);
  });

  // Two rungs that round to the same label are indistinguishable on screen.
  it("keeps one rung per tier", () => {
    expect(qualityRungsFromHeights([1080, 1040, 720])).toEqual([
      { value: 1080, tier: 1080, label: "1080p" },
      { value: 720, tier: 720, label: "720p" },
    ]);
  });

  it("offers nothing when there is nothing, so the caller keeps its ladder", () => {
    expect(qualityRungsFromHeights([])).toEqual([]);
    expect(qualityRungsFromHeights([0, -1])).toEqual([]);
  });
});
