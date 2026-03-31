import { describe, it, expect } from "vitest";
import { artworkUrl } from "./artwork";

describe("artworkUrl", () => {
  it("builds primary artwork URL by default", () => {
    expect(artworkUrl("https://evey.test", "abc-123")).toBe(
      "https://evey.test/api/v1/media/abc-123/artwork/primary",
    );
  });

  it("builds primary artwork URL when explicit", () => {
    expect(artworkUrl("https://evey.test", "m1", "primary")).toBe(
      "https://evey.test/api/v1/media/m1/artwork/primary",
    );
  });

  it("builds backdrop artwork URL", () => {
    expect(artworkUrl("https://evey.test", "m2", "backdrop")).toBe(
      "https://evey.test/api/v1/media/m2/artwork/backdrop",
    );
  });

  it("handles trailing slash on server URL", () => {
    expect(artworkUrl("https://evey.test/", "m3")).toBe(
      "https://evey.test//api/v1/media/m3/artwork/primary",
    );
  });

  it("handles empty mediaId", () => {
    expect(artworkUrl("https://evey.test", "")).toBe(
      "https://evey.test/api/v1/media//artwork/primary",
    );
  });
});
