import { describe, it, expect } from "vitest";
import { artworkUrl } from "./artwork";

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

  it("appends width query param when set", () => {
    expect(artworkUrl("https://evey.test", "m4", "poster", 200)).toBe(
      "https://evey.test/api/v1/media/m4/artwork/poster?w=200",
    );
  });

  it("appends width on backdrop too", () => {
    expect(artworkUrl("https://evey.test", "m5", "backdrop", 1280)).toBe(
      "https://evey.test/api/v1/media/m5/artwork/backdrop?w=1280",
    );
  });

  it("percent-encodes a traversal/redirect id so the fetch cannot pivot to another API path", () => {
    expect(artworkUrl("https://evey.test", "../admin/api-keys")).toBe(
      "https://evey.test/api/v1/media/..%2Fadmin%2Fapi-keys/artwork/poster",
    );
  });

  it("ignores width <= 0", () => {
    expect(artworkUrl("https://evey.test", "m6", "poster", 0)).toBe(
      "https://evey.test/api/v1/media/m6/artwork/poster",
    );
    expect(artworkUrl("https://evey.test", "m7", "poster", -1)).toBe(
      "https://evey.test/api/v1/media/m7/artwork/poster",
    );
  });
});
