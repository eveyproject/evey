import { describe, expect, it } from "vitest";
import { clientSubtitleTrack } from "./clientSubtitleTrack";

// A realistic file: video, audio, then two subtitles. The absolute indices and
// the subtitle-relative ones differ by two - which is exactly the gap the bug
// fell through.
const STREAMS = [
  { index: 2, trackIndex: 0 },
  { index: 3, trackIndex: 1 },
];

describe("clientSubtitleTrack", () => {
  it("returns the SUBTITLE-relative index, not the absolute stream index", () => {
    expect(clientSubtitleTrack(2, STREAMS)).toBe(0);
    expect(clientSubtitleTrack(3, STREAMS)).toBe(1);
  });

  it("returns null for a burn-in track - the server already drew it", () => {
    expect(clientSubtitleTrack(2, [{ index: 2, trackIndex: 0, burnIn: true }])).toBeNull();
  });

  it("returns null when subtitles are off", () => {
    expect(clientSubtitleTrack(null, STREAMS)).toBeNull();
  });

  // Rather than fall back to the raw number, which is how the wrong track got
  // fetched in the first place.
  it("returns null for a selection no stream matches", () => {
    expect(clientSubtitleTrack(9, STREAMS)).toBeNull();
  });

  // The one arrangement where the bug was invisible: subtitles first, so the two
  // indices coincide. Pinned so nobody "simplifies" the helper back by testing
  // only this shape.
  it("agrees with the absolute index only when subtitles start at zero", () => {
    const flat = [{ index: 0, trackIndex: 0 }, { index: 1, trackIndex: 1 }];
    expect(clientSubtitleTrack(1, flat)).toBe(1);
  });
});
