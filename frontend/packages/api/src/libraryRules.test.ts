import { describe, it, expect } from "vitest";
import {
  DEDICATED_ROUTES,
  dedicatedRouteFor,
  hasMediaItemCount,
  defaultTypeFilterFor,
  shouldAutoFetchMore,
  AUTO_FETCH_FILL_TARGET,
  AUTO_FETCH_ITEM_CAP,
} from "./libraryRules";

// Route sets as each app actually implements them today.
const WEB_ROUTES = ["/photos", "/music"];
const MOBILE_ROUTES = ["/photos", "/music"];
const TV_ROUTES = ["/music"]; // no photos screen on TV

describe("dedicatedRouteFor", () => {
  // The 2026-07-28 bug: a music library browsed through the generic media grid
  // shows an empty screen, because its tracks are not media_items at all.
  // Verified against production: /api/v1/media?libraryId=<music> returns 0
  // items while /api/v1/music/albums?libraryId=<same> returns 3 albums.
  it.each([
    ["web", WEB_ROUTES],
    ["mobile", MOBILE_ROUTES],
  ])("sends a music library to /music on %s", (_app, routes) => {
    expect(dedicatedRouteFor("LIBRARY_TYPE_MUSIC", routes)).toBe("/music");
  });

  it.each([
    ["web", WEB_ROUTES],
    ["mobile", MOBILE_ROUTES],
  ])("sends a photo library to /photos on %s", (_app, routes) => {
    expect(dedicatedRouteFor("LIBRARY_TYPE_PHOTOS", routes)).toBe("/photos");
  });

  it("still routes music on TV, which has that screen", () => {
    expect(dedicatedRouteFor("LIBRARY_TYPE_MUSIC", TV_ROUTES)).toBe("/music");
  });

  // A redirect to a screen the app does not have would swap an empty grid for
  // a dead end, so an unsupported route must fall back to normal browsing.
  it("does not send TV to a photos screen it does not implement", () => {
    expect(dedicatedRouteFor("LIBRARY_TYPE_PHOTOS", TV_ROUTES)).toBeUndefined();
  });

  it.each(["LIBRARY_TYPE_MOVIES", "LIBRARY_TYPE_SHOWS", "LIBRARY_TYPE_UNSPECIFIED", undefined, ""])(
    "browses %s through the normal media grid",
    (type) => {
      expect(dedicatedRouteFor(type, WEB_ROUTES)).toBeUndefined();
    },
  );
});

describe("hasMediaItemCount", () => {
  // The mobile libraries list showed "0 items" under Musique while the library
  // held 3 albums, because the count came from ListMediaItems.
  it.each(["LIBRARY_TYPE_MUSIC", "LIBRARY_TYPE_PHOTOS"])("is false for %s", (type) => {
    expect(hasMediaItemCount(type)).toBe(false);
  });

  it.each(["LIBRARY_TYPE_MOVIES", "LIBRARY_TYPE_SHOWS", undefined])("is true for %s", (type) => {
    expect(hasMediaItemCount(type)).toBe(true);
  });

  it("covers exactly the dedicated-route types, so the two rules cannot drift", () => {
    for (const type of Object.keys(DEDICATED_ROUTES)) {
      expect(hasMediaItemCount(type)).toBe(false);
    }
  });
});

describe("defaultTypeFilterFor", () => {
  it("opens a shows library on series only", () => {
    expect(defaultTypeFilterFor("LIBRARY_TYPE_SHOWS")).toBe("MEDIA_ITEM_TYPE_SERIES");
  });

  it("opens a movies library on movies only", () => {
    expect(defaultTypeFilterFor("LIBRARY_TYPE_MOVIES")).toBe("MEDIA_ITEM_TYPE_MOVIE");
  });

  it.each(["LIBRARY_TYPE_UNSPECIFIED", undefined, ""])("has no default for %s", (type) => {
    expect(defaultTypeFilterFor(type)).toBe("");
  });
});

describe("shouldAutoFetchMore", () => {
  const base = {
    hasLocalFilters: true,
    hasMore: true,
    isFetching: false,
    filteredCount: 0,
    loadedCount: 0,
  };

  it("fetches while a client-only facet leaves the grid too short", () => {
    expect(shouldAutoFetchMore(base)).toBe(true);
  });

  it.each([
    ["no local filter is active", { hasLocalFilters: false }],
    ["the list has no more pages", { hasMore: false }],
    ["a fetch is already in flight", { isFetching: true }],
  ])("does not fetch when %s", (_why, override) => {
    expect(shouldAutoFetchMore({ ...base, ...override })).toBe(false);
  });

  it("stops once enough matches are visible", () => {
    expect(shouldAutoFetchMore({ ...base, filteredCount: AUTO_FETCH_FILL_TARGET })).toBe(false);
  });

  // Without this cap, a facet matching nothing would walk the entire library
  // one page at a time - 270 requests on the 8076-item Séries library.
  it("stops at the item cap even with nothing matching", () => {
    expect(shouldAutoFetchMore({ ...base, loadedCount: AUTO_FETCH_ITEM_CAP })).toBe(false);
  });
});
