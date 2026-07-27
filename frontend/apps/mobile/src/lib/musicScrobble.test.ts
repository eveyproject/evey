import { describe, it, expect } from "vitest";
import { shouldRecordScrobble } from "./musicScrobble";

describe("shouldRecordScrobble", () => {
  it("returns false when the track hasn't finished", () => {
    expect(shouldRecordScrobble({ didJustFinish: false, trackId: "t1", lastRecordedTrackId: null })).toBe(false);
  });

  it("returns false when there is no active track", () => {
    expect(shouldRecordScrobble({ didJustFinish: true, trackId: null, lastRecordedTrackId: null })).toBe(false);
  });

  it("returns true the first time a track finishes", () => {
    expect(shouldRecordScrobble({ didJustFinish: true, trackId: "t1", lastRecordedTrackId: null })).toBe(true);
  });

  it("returns false for a duplicate finished tick on the same track", () => {
    expect(shouldRecordScrobble({ didJustFinish: true, trackId: "t1", lastRecordedTrackId: "t1" })).toBe(false);
  });

  it("returns true again once a different track finishes", () => {
    expect(shouldRecordScrobble({ didJustFinish: true, trackId: "t2", lastRecordedTrackId: "t1" })).toBe(true);
  });
});
