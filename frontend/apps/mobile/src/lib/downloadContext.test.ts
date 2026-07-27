import { describe, it, expect } from "vitest";
import { buildDownloadAuthContext } from "./downloadContext";

describe("buildDownloadAuthContext", () => {
  it("captures an adult profile's identity alongside the content classification", () => {
    const ctx = buildDownloadAuthContext(
      { id: "adult-1", isKids: false, maxAge: undefined, region: "US" },
      { officialRating: "R", certificationsByCountry: { FR: "16" } },
    );
    expect(ctx).toEqual({
      downloadedByProfileId: "adult-1",
      downloadedByIsKids: false,
      downloadedByMaxAge: undefined,
      officialRating: "R",
      certificationsByCountry: { FR: "16" },
    });
  });

  it("captures a kids profile's maxAge", () => {
    const ctx = buildDownloadAuthContext(
      { id: "kid-1", isKids: true, maxAge: 10 },
      { officialRating: "PG" },
    );
    expect(ctx.downloadedByProfileId).toBe("kid-1");
    expect(ctx.downloadedByIsKids).toBe(true);
    expect(ctx.downloadedByMaxAge).toBe(10);
    expect(ctx.officialRating).toBe("PG");
  });

  it("omits the downloader identity when no active profile is resolvable", () => {
    const ctx = buildDownloadAuthContext(null, { officialRating: "PG-13" });
    expect(ctx.downloadedByProfileId).toBeUndefined();
    expect(ctx.downloadedByIsKids).toBeUndefined();
    expect(ctx.downloadedByMaxAge).toBeUndefined();
    expect(ctx.officialRating).toBe("PG-13");
  });

  it("treats a null profile id the same as no profile at all", () => {
    const ctx = buildDownloadAuthContext({ id: null, isKids: true, maxAge: 8 }, undefined);
    expect(ctx.downloadedByProfileId).toBeUndefined();
  });

  it("omits classification fields when the content carries none (e.g. a music track)", () => {
    const ctx = buildDownloadAuthContext({ id: "adult-1", isKids: false }, undefined);
    expect(ctx.officialRating).toBeUndefined();
    expect(ctx.certificationsByCountry).toBeUndefined();
    expect(ctx.downloadedByProfileId).toBe("adult-1");
  });
});
