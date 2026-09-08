import { describe, expect, it } from "vitest";
import { burnInSubtitleIndex, clientSubtitleTrack } from "./clientSubtitleTrack";

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

describe("burnInSubtitleIndex", () => {
  // The a 4K remux 4K remux of the 2026-09-08 incident: a PGS track the
  // file flags default, auto-selected by pickDefaultSubtitle without ever
  // restarting the session for it. The stream is direct-playing with nothing
  // burnt in, so a restart for an unrelated reason must not send the index -
  // that is what flipped the copy rung to a four-rung tonemapped ladder.
  const PGS = [{ index: 2, trackIndex: 0, burnIn: true }];
  const TEXT = [{ index: 2, trackIndex: 0 }];

  it("omits a text subtitle - SubtitleOverlay draws it, the server must not", () => {
    expect(burnInSubtitleIndex(2, TEXT)).toBeUndefined();
  });

  it("omits it when subtitles are off", () => {
    expect(burnInSubtitleIndex(null, PGS)).toBeUndefined();
  });

  it("omits a selection no stream matches rather than passing the raw number", () => {
    expect(burnInSubtitleIndex(9, PGS)).toBeUndefined();
  });

  // The burn-in the viewer DID ask for has to survive an audio or quality
  // change: dropping it would silently turn their subtitles off mid-film.
  it("keeps a bitmap subtitle, which is the only kind the server can burn", () => {
    expect(burnInSubtitleIndex(2, PGS)).toBe(2);
  });

  // The pair is exhaustive by construction: whichever half answers, the other
  // one declines. A track that both drew would double every line; one that
  // neither drew would be selected in the UI and invisible on screen.
  it("is the exact complement of clientSubtitleTrack", () => {
    for (const streams of [PGS, TEXT]) {
      const drawnByServer = burnInSubtitleIndex(2, streams) !== undefined;
      const drawnByClient = clientSubtitleTrack(2, streams) !== null;
      expect(drawnByServer).not.toBe(drawnByClient);
    }
  });
});
