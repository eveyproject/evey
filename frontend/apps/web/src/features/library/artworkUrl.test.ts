import { describe, it, expect } from "vitest";
import { artworkUrl, artworkVersion } from "./artworkUrl";

describe("artworkVersion", () => {
  it("derives a compact epoch from updatedAt", () => {
    const v = artworkVersion({ updatedAt: "2026-06-20T12:00:00Z" });
    expect(v).toBe(String(Date.parse("2026-06-20T12:00:00Z")));
  });

  it("falls back to createdAt when updatedAt is absent", () => {
    expect(artworkVersion({ createdAt: "2026-01-01T00:00:00Z" })).toBe(
      String(Date.parse("2026-01-01T00:00:00Z")),
    );
  });

  it("is empty when no timestamp is known (URL stays unversioned)", () => {
    expect(artworkVersion(undefined)).toBe("");
    expect(artworkVersion({})).toBe("");
  });

  it("is empty for an unparseable timestamp rather than NaN", () => {
    expect(artworkVersion({ updatedAt: "not-a-date" })).toBe("");
  });
});

describe("artworkUrl", () => {
  const item = { updatedAt: "2026-06-20T12:00:00Z" };
  const ver = String(Date.parse("2026-06-20T12:00:00Z"));

  it("appends the content version so the immutable cache busts on change", () => {
    expect(artworkUrl("mi-1", "poster", item)).toBe(`/api/v1/media/mi-1/artwork/poster?v=${ver}`);
  });

  it("omits ?v= entirely when the version is unknown", () => {
    expect(artworkUrl("mi-2", "backdrop", undefined)).toBe("/api/v1/media/mi-2/artwork/backdrop");
  });

  it("never carries a token: the artwork route is public", () => {
    expect(artworkUrl("mi-3", "poster", item)).not.toContain("token=");
  });
});
