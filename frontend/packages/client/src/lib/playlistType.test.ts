import { describe, it, expect } from "vitest";
import { isSmartPlaylist } from "./playlistType";

describe("isSmartPlaylist", () => {
  it("recognises both spellings the API and the local type disagree on", () => {
    expect(isSmartPlaylist("PLAYLIST_TYPE_SMART")).toBe(true);
    expect(isSmartPlaylist("smart")).toBe(true);
  });

  it("treats everything else as a manual playlist", () => {
    expect(isSmartPlaylist("PLAYLIST_TYPE_MANUAL")).toBe(false);
    expect(isSmartPlaylist("manual")).toBe(false);
    expect(isSmartPlaylist(undefined)).toBe(false);
  });
});
