import { describe, it, expect } from "vitest";
import { coverArtUri } from "./coverArtUri";

describe("coverArtUri", () => {
  it("prefixes the server and asks for a density-scaled width", () => {
    const uri = coverArtUri("http://srv", "/api/v1/music/albums/al-1/art", 48);
    expect(uri).toMatch(/^http:\/\/srv\/api\/v1\/music\/albums\/al-1\/art\?w=\d+$/);
    expect(Number(uri!.split("w=")[1])).toBeGreaterThanOrEqual(48);
  });
  it("extends an existing query string", () => {
    expect(coverArtUri("http://srv", "/art?token=t", 10)).toMatch(/^http:\/\/srv\/art\?token=t&w=\d+$/);
  });
  it("leaves provider-hosted art alone", () => {
    expect(coverArtUri("http://srv", "https://cdn.example/c.jpg", 48)).toBe("https://cdn.example/c.jpg");
  });
  it("is undefined without art", () => {
    expect(coverArtUri("http://srv", undefined, 48)).toBeUndefined();
  });
});
