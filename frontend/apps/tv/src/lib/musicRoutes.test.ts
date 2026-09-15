import { describe, it, expect } from "vitest";
import { musicAlbumHref, musicArtistHref } from "./musicRoutes";

// Album and artist screens are scoped by libraryId server-side: every link
// must carry it, and ids that contain slashes or spaces must survive routing.
describe("music route builders", () => {
  it("carries the library id along", () => {
    expect(musicArtistHref("artist-1", "lib-1")).toBe("/(auth)/music/artist/artist-1?libraryId=lib-1");
    expect(musicAlbumHref("album-1", "lib-1")).toBe("/(auth)/music/album/album-1?libraryId=lib-1");
  });

  it("encodes ids so a slash or space cannot break the path or the query", () => {
    expect(musicArtistHref("AC/DC", "lib 1")).toBe("/(auth)/music/artist/AC%2FDC?libraryId=lib%201");
    expect(musicAlbumHref("a&b=c", "l#1")).toBe("/(auth)/music/album/a%26b%3Dc?libraryId=l%231");
  });
});
