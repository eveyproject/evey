import { describe, it, expect } from "vitest";
import { coverArtSrc } from "./coverArtSrc";

describe("coverArtSrc", () => {
  it("appends the width after a signed token", () => {
    expect(coverArtSrc("/api/v1/music/albums/al-1/art", "?token=abc", 400)).toBe("/api/v1/music/albums/al-1/art?token=abc&w=400");
  });
  it("starts the query string when there is no token", () => {
    expect(coverArtSrc("/api/v1/music/albums/al-1/art", "", 96)).toBe("/api/v1/music/albums/al-1/art?w=96");
  });
  it("leaves provider-hosted art alone", () => {
    expect(coverArtSrc("https://cdn.example/x.jpg", "?token=abc", 400)).toBe("https://cdn.example/x.jpg");
  });
  it("is undefined without a url", () => {
    expect(coverArtSrc(undefined, "?token=abc", 400)).toBeUndefined();
  });
});
