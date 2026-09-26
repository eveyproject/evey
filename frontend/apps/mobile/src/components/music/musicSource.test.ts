import { describe, it, expect } from "vitest";
import {
  albumMusicSource,
  artistMusicSource,
  trackListMusicSource,
  searchTrackMusicSource,
  restoreMusicFilterFromParams,
} from "./musicSource";

describe("albumMusicSource", () => {
  it("builds an album source with the given label", () => {
    expect(albumMusicSource("a1", "Abbey Road")).toEqual({ type: "album", id: "a1", label: "Abbey Road" });
  });

  it("falls back to the id when the label is empty", () => {
    expect(albumMusicSource("a1", "")).toEqual({ type: "album", id: "a1", label: "a1" });
  });
});

describe("artistMusicSource", () => {
  it("uses the artist name as both id and label", () => {
    expect(artistMusicSource("The Lanterns")).toEqual({ type: "artist", id: "The Lanterns", label: "The Lanterns" });
  });
});

describe("trackListMusicSource", () => {
  it("returns an album source when an album filter is active", () => {
    expect(trackListMusicSource("a1", "Abbey Road", null)).toEqual({ type: "album", id: "a1", label: "Abbey Road" });
  });

  it("returns an artist source when an artist filter is active", () => {
    expect(trackListMusicSource(null, "", "The Lanterns")).toEqual({ type: "artist", id: "The Lanterns", label: "The Lanterns" });
  });

  it("prefers the album filter when both are somehow set", () => {
    expect(trackListMusicSource("a1", "Abbey Road", "The Lanterns")).toEqual({ type: "album", id: "a1", label: "Abbey Road" });
  });

  it("returns null for an unfiltered listing", () => {
    expect(trackListMusicSource(null, "", null)).toBeNull();
  });
});

describe("searchTrackMusicSource", () => {
  it("returns an album source when the track carries an albumId", () => {
    expect(searchTrackMusicSource("a1", "Abbey Road")).toEqual({ type: "album", id: "a1", label: "Abbey Road" });
  });

  it("falls back to the album id as label when no label is given", () => {
    expect(searchTrackMusicSource("a1", undefined)).toEqual({ type: "album", id: "a1", label: "a1" });
  });

  it("returns null when the track has no albumId", () => {
    expect(searchTrackMusicSource(undefined, "Abbey Road")).toBeNull();
  });
});

describe("restoreMusicFilterFromParams", () => {
  it("returns null when no relevant param is present", () => {
    expect(restoreMusicFilterFromParams({})).toBeNull();
  });

  it("restores a playlist target", () => {
    expect(restoreMusicFilterFromParams({ playlistId: "p1" })).toEqual({
      tab: "playlists",
      albumFilter: null,
      albumFilterLabel: "",
      artistFilter: null,
      playlistId: "p1",
    });
  });

  it("restores an album filter with its label", () => {
    expect(restoreMusicFilterFromParams({ album: "a1", albumLabel: "Abbey Road" })).toEqual({
      tab: "tracks",
      albumFilter: "a1",
      albumFilterLabel: "Abbey Road",
      artistFilter: null,
      playlistId: null,
    });
  });

  it("falls back to the album id as label when albumLabel is missing", () => {
    expect(restoreMusicFilterFromParams({ album: "a1" })?.albumFilterLabel).toBe("a1");
  });

  it("restores an artist filter", () => {
    expect(restoreMusicFilterFromParams({ artist: "The Lanterns" })).toEqual({
      tab: "tracks",
      albumFilter: null,
      albumFilterLabel: "",
      artistFilter: "The Lanterns",
      playlistId: null,
    });
  });

  it("prioritizes playlistId over album/artist when several are present", () => {
    expect(restoreMusicFilterFromParams({ playlistId: "p1", album: "a1", artist: "The Lanterns" })?.tab).toBe("playlists");
  });

  it("falls back to a bare tab switch with no filter", () => {
    expect(restoreMusicFilterFromParams({ tab: "albums" })).toEqual({
      tab: "albums",
      albumFilter: null,
      albumFilterLabel: "",
      artistFilter: null,
      playlistId: null,
    });
  });

  it("rejects an unknown tab value and defaults to tracks", () => {
    expect(restoreMusicFilterFromParams({ tab: "bogus" })?.tab).toBe("tracks");
  });
});
