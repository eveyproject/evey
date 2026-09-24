import { describe, it, expect } from "vitest";
import { artworkUrl, coverArtUri } from "./artwork";

describe("artworkUrl", () => {
  it("builds poster artwork URL by default", () => {
    expect(artworkUrl("https://evey.test", "abc-123")).toBe(
      "https://evey.test/api/v1/media/abc-123/artwork/poster",
    );
  });

  it("builds poster artwork URL when explicit", () => {
    expect(artworkUrl("https://evey.test", "m1", "poster")).toBe(
      "https://evey.test/api/v1/media/m1/artwork/poster",
    );
  });

  it("builds backdrop artwork URL", () => {
    expect(artworkUrl("https://evey.test", "m2", "backdrop")).toBe(
      "https://evey.test/api/v1/media/m2/artwork/backdrop",
    );
  });

  it("handles trailing slash on server URL", () => {
    expect(artworkUrl("https://evey.test/", "m3")).toBe(
      "https://evey.test//api/v1/media/m3/artwork/poster",
    );
  });

  it("handles empty mediaId", () => {
    expect(artworkUrl("https://evey.test", "")).toBe(
      "https://evey.test/api/v1/media//artwork/poster",
    );
  });

  it("appends the width query param for row/grid thumbnails", () => {
    expect(artworkUrl("https://evey.test", "m4", "poster", 220)).toBe(
      "https://evey.test/api/v1/media/m4/artwork/poster?w=220",
    );
  });

  it("omits the width param when 0 or undefined", () => {
    expect(artworkUrl("https://evey.test", "m5", "poster", 0)).toBe(
      "https://evey.test/api/v1/media/m5/artwork/poster",
    );
    expect(artworkUrl("https://evey.test", "m5", "backdrop")).toBe(
      "https://evey.test/api/v1/media/m5/artwork/backdrop",
    );
  });
});

describe("coverArtUri", () => {
  it("returns undefined without art", () => {
    expect(coverArtUri("http://srv", undefined)).toBeUndefined();
    expect(coverArtUri("http://srv", "")).toBeUndefined();
  });

  it("prefixes a server path and asks the server for the display width", () => {
    expect(coverArtUri("http://srv", "/api/v1/music/albums/al-1/art", 160)).toBe(
      "http://srv/api/v1/music/albums/al-1/art?w=160",
    );
  });

  it("extends an existing query string instead of clobbering it", () => {
    expect(coverArtUri("http://srv", "/api/v1/music/albums/al-1/art?token=abc", 160)).toBe(
      "http://srv/api/v1/music/albums/al-1/art?token=abc&w=160",
    );
  });

  it("leaves provider-hosted art untouched: nothing resizes it", () => {
    expect(coverArtUri("http://srv", "https://cdn.example/cover.jpg", 160)).toBe("https://cdn.example/cover.jpg");
  });

  it("omits the width when none or a non-positive one is given", () => {
    expect(coverArtUri("http://srv", "/art")).toBe("http://srv/art");
    expect(coverArtUri("http://srv", "/art", 0)).toBe("http://srv/art");
  });
});

// Artwork is served immutable for a year (measured on the dev server): a
// replaced poster reaches a client only through a new URL.
describe("artworkUrl versioning", () => {
  it("adds the item's change time as ?v= before the width", () => {
    const item = { updatedAt: "2026-09-24T10:00:00Z" };
    expect(artworkUrl("http://srv", "m1", "poster", 400, item)).toBe(
      `http://srv/api/v1/media/m1/artwork/poster?v=${Date.parse(item.updatedAt)}&w=400`,
    );
  });

  it("stays unversioned when the item's change time is unknown", () => {
    expect(artworkUrl("http://srv", "m1", "poster", 400, {})).toBe("http://srv/api/v1/media/m1/artwork/poster?w=400");
    expect(artworkUrl("http://srv", "m1", "backdrop")).toBe("http://srv/api/v1/media/m1/artwork/backdrop");
  });
});
