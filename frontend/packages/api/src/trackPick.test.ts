import { describe, it, expect } from "vitest";
import { parseTrackPick, trackPickParams, trackPickQuery } from "./trackPick";

describe("parseTrackPick", () => {
  it("reads string indices from a URL and numbers from a typed router", () => {
    expect(parseTrackPick({ audio: "3", subtitle: "5" })).toEqual({ audio: 3, subtitle: 5 });
    expect(parseTrackPick({ audio: 2, subtitle: 0 })).toEqual({ audio: 2, subtitle: 0 });
  });

  it("keeps the explicit off choice apart from an absent one", () => {
    expect(parseTrackPick({ subtitle: "off" })).toEqual({ subtitle: "off" });
    expect(parseTrackPick({})).toEqual({});
    expect(parseTrackPick(undefined)).toEqual({});
  });

  it("drops garbage instead of feeding it to the player", () => {
    expect(parseTrackPick({ audio: "-1", subtitle: "abc" })).toEqual({});
    expect(parseTrackPick({ audio: 1.5, subtitle: true })).toEqual({});
    expect(parseTrackPick({ audio: "3; DROP", subtitle: ["off"] })).toEqual({});
  });
});

describe("trackPickParams / trackPickQuery", () => {
  it("round-trips through the query string", () => {
    const pick = { audio: 3, subtitle: "off" as const };
    const query = trackPickQuery(pick);
    expect(query).toBe("audio=3&subtitle=off");
    expect(parseTrackPick(Object.fromEntries(new URLSearchParams(query)))).toEqual(pick);
  });

  it("emits nothing for an empty pick so a plain play URL stays byte-identical", () => {
    expect(trackPickParams(undefined)).toEqual({});
    expect(trackPickQuery({})).toBe("");
    expect(trackPickParams({ subtitle: 0 })).toEqual({ subtitle: "0" });
  });
});
