import { describe, it, expect } from "vitest";
import type { V1Trailer } from "@evey/api";
import { trailerUrl, sortTrailers } from "./trailerUrl";

describe("trailerUrl", () => {
  it("builds the watch links the other clients build", () => {
    expect(trailerUrl({ site: "YouTube", key: "abc123" })).toBe("https://www.youtube.com/watch?v=abc123");
    expect(trailerUrl({ site: "Vimeo", key: "987" })).toBe("https://vimeo.com/987");
  });

  it("is case-insensitive about the site, which the metadata provider is not", () => {
    expect(trailerUrl({ site: "youtube", key: "x" })).toBe("https://www.youtube.com/watch?v=x");
    expect(trailerUrl({ site: "YOUTUBE", key: "x" })).toBe("https://www.youtube.com/watch?v=x");
  });

  // A button that leads nowhere is worse on a remote than an absent one: the
  // D-pad lands on it, the viewer presses OK, and nothing happens.
  it("returns null for anything it cannot build a link for", () => {
    expect(trailerUrl({ site: "Dailymotion", key: "x" })).toBeNull();
    expect(trailerUrl({ site: "YouTube" })).toBeNull();
    expect(trailerUrl({ key: "abc" })).toBeNull();
    expect(trailerUrl({})).toBeNull();
  });
});

describe("sortTrailers", () => {
  it("puts the official trailer first, then anything typed Trailer, then the rest", () => {
    const teaser: V1Trailer = { site: "YouTube", key: "teaser", type: "Teaser" };
    const unofficial: V1Trailer = { site: "YouTube", key: "unofficial", type: "Trailer" };
    const official: V1Trailer = { site: "YouTube", key: "official", type: "Trailer", official: true };

    expect(sortTrailers([teaser, unofficial, official]).map((t) => t.key)).toEqual([
      "official",
      "unofficial",
      "teaser",
    ]);
  });

  it("does not mutate the caller's array", () => {
    const list: V1Trailer[] = [
      { site: "YouTube", key: "a", type: "Teaser" },
      { site: "YouTube", key: "b", official: true, type: "Trailer" },
    ];
    sortTrailers(list);
    expect(list.map((t) => t.key)).toEqual(["a", "b"]);
  });
});
