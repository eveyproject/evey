import { describe, it, expect } from "vitest";
import { describeCastTracks, shouldReportTracks } from "./castTrackBeacon";

describe("describeCastTracks", () => {
  it("reports nothing when the receiver exposed no tracks", () => {
    // The caller skips the beacon on "" - an empty list is the normal state
    // before the receiver has parsed the manifest, not a finding.
    expect(describeCastTracks(undefined, undefined)).toBe("");
    expect(describeCastTracks([], [1])).toBe("");
  });

  it("renders id, kind, language and name as the receiver spelled them", () => {
    const line = describeCastTracks(
      [{ id: 1, type: "audio", language: "fr", name: "Français (AC-3)" }],
      undefined,
    );
    expect(line).toBe("count=1 1:audio:fr Français (AC-3)");
  });

  it("marks which tracks the receiver has active", () => {
    const line = describeCastTracks(
      [
        { id: 1, type: "audio", language: "en" },
        { id: 2, type: "text", subtype: "subtitles", language: "fr" },
      ],
      [2],
    );
    expect(line).toContain("1:audio:en");
    expect(line).toContain("2:text/subtitles:fr:ACTIVE");
    expect(line).toContain("count=2");
  });

  it("survives a track with neither language nor name", () => {
    expect(describeCastTracks([{ id: 7, type: "video" }], [])).toBe("count=1 7:video");
  });
});

describe("shouldReportTracks", () => {
  it("reports the first non-empty line", () => {
    expect(shouldReportTracks(null, "count=1 1:audio")).toBe(true);
  });

  it("does not re-report an unchanged shape", () => {
    // The status stream fires several times a second; the track list is a
    // property of the media, not of the moment.
    expect(shouldReportTracks("count=1 1:audio", "count=1 1:audio")).toBe(false);
  });

  it("reports again when the shape changes", () => {
    // The receiver gaining renditions after parsing the manifest is real news.
    expect(shouldReportTracks("count=1 1:audio", "count=2 1:audio | 2:text")).toBe(true);
  });

  it("never reports an empty line", () => {
    expect(shouldReportTracks(null, "")).toBe(false);
    expect(shouldReportTracks("count=1 1:audio", "")).toBe(false);
  });
});
