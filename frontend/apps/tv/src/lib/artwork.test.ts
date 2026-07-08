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
