import { describe, expect, it } from "vitest";
import { parseHash, routeToHash, type Route } from "./router";

// Every screen is reachable by its address and comes back the same: the
// packaged application has no server to rewrite anything, so the hash IS the
// navigation.
const ROUTES: Route[] = [
  { name: "home" },
  { name: "server" },
  { name: "login" },
  { name: "quickconnect" },
  { name: "profiles" },
  { name: "settings" },
  { name: "libraries" },
  { name: "collections" },
  { name: "playlists" },
  { name: "genres" },
  { name: "favorites" },
  { name: "history" },
  { name: "notifications" },
  { name: "requests" },
  { name: "syncplay" },
  { name: "servers" },
  { name: "stats" },
  { name: "playback" },
  { name: "library", libraryId: "lib-1" },
  { name: "media", mediaId: "m-1" },
  { name: "collection", collectionId: "c-1" },
  { name: "playlist", playlistId: "p-1" },
  { name: "genre", genre: "Science-Fiction" },
  { name: "person", personId: "per-1" },
  { name: "plugin-settings", pluginId: "plg-1" },
  { name: "album", albumId: "a-1" },
  { name: "artist", artistId: "art-1" },
  { name: "search", query: "blade runner" },
  { name: "music", libraryId: "lib-music" },
  { name: "photos", libraryId: "lib-photos", albumId: "Holidays" },
  { name: "slideshow", libraryId: "lib-photos", albumId: "Holidays", index: 4 },
  { name: "player", mediaId: "m-1", restart: true, audio: 2, subtitle: 4, roomId: "room-7" },
];

describe("router", () => {
  it.each(ROUTES)("round-trips $name", (route) => {
    expect(parseHash(routeToHash(route))).toEqual(route);
  });

  it("escapes a value that would otherwise split the address", () => {
    const route: Route = { name: "genre", genre: "Action & Adventure/Other" };
    expect(parseHash(routeToHash(route))).toEqual(route);
  });

  it("falls back to the home rather than a screen with nothing to show", () => {
    expect(parseHash("#/media")).toEqual({ name: "home" });
    expect(parseHash("#/player")).toEqual({ name: "home" });
    expect(parseHash("#/nonsense")).toEqual({ name: "home" });
    expect(parseHash("#/collection")).toEqual({ name: "collections" });
    expect(parseHash("#/playlist")).toEqual({ name: "playlists" });
    expect(parseHash("#/genre")).toEqual({ name: "genres" });
  });

  it("reads a subtitle turned off, which is not the same as none chosen", () => {
    expect(parseHash("#/player/m-1?subtitle=off")).toEqual({ name: "player", mediaId: "m-1", restart: false, subtitle: "off" });
  });
});
