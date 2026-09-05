import { describe, it, expect } from "vitest";
import { canPlayOffline, OFFLINE_DOWNLOAD_TTL_MS, type OfflineProfileInfo } from "./offlineAccess";
import type { DownloadedMedia } from "./downloads";

const NOW = Date.parse("2026-07-28T12:00:00.000Z");

function entry(overrides: Partial<DownloadedMedia> = {}): DownloadedMedia {
  return {
    mediaItemId: "m1",
    title: "Movie",
    filePath: "/downloads/m1.mp4",
    fileSize: 1000,
    downloadedAt: "2026-07-01T00:00:00.000Z",
    ...overrides,
  };
}

const adult: OfflineProfileInfo = { id: "adult-1", isKids: false };
const kid10: OfflineProfileInfo = { id: "kid-1", isKids: true, maxAge: 10, region: "US" };

describe("canPlayOffline", () => {
  it("allows an adult profile replaying its own download with no rating info (offline flight scenario)", () => {
    const e = entry({ downloadedByProfileId: "adult-1" });
    expect(canPlayOffline(e, adult, NOW)).toEqual({ allowed: true });
  });

  it("allows an adult profile replaying a download it made even when the title is rated for adults", () => {
    const e = entry({ downloadedByProfileId: "adult-1", officialRating: "R" });
    expect(canPlayOffline(e, adult, NOW)).toEqual({ allowed: true });
  });

  it("refuses a kids profile whose age cap is exceeded by the content classification", () => {
    const e = entry({ downloadedByProfileId: "adult-1", officialRating: "R" });
    expect(canPlayOffline(e, kid10, NOW)).toEqual({ allowed: false, reason: "age-gated" });
  });

  it("allows a kids profile replaying its own download when the content classification is under its age cap", () => {
    const e = entry({ downloadedByProfileId: kid10.id, downloadedByIsKids: true, downloadedByMaxAge: 10, officialRating: "G" });
    expect(canPlayOffline(e, kid10, NOW)).toEqual({ allowed: true });
  });

  it("allows a kids profile replaying its own download when the content has no classification at all (defer to server, matches useParentalGate philosophy)", () => {
    const e = entry({ downloadedByProfileId: kid10.id, downloadedByIsKids: true, downloadedByMaxAge: 10 });
    expect(canPlayOffline(e, kid10, NOW)).toEqual({ allowed: true });
  });

  it("refuses a different, more restricted profile even for unrated content downloaded by a less restricted one", () => {
    const e = entry({ downloadedByProfileId: "adult-1", downloadedByIsKids: false });
    expect(canPlayOffline(e, kid10, NOW)).toEqual({ allowed: false, reason: "more-restricted-profile" });
  });

  it("allows a different profile that is NOT more restricted than the one who downloaded it", () => {
    const otherAdult: OfflineProfileInfo = { id: "adult-2", isKids: false };
    const e = entry({ downloadedByProfileId: "adult-1", downloadedByIsKids: false });
    expect(canPlayOffline(e, otherAdult, NOW)).toEqual({ allowed: true });
  });

  // A cold start with no network is now a supported way into the app, and the
  // profile list is exactly what never arrives then: `isKids: undefined` there
  // means "nobody could tell us", not "not a kids profile".
  describe("when the active profile's restriction could not be resolved", () => {
    const unknown: OfflineProfileInfo = { id: "kid-1", restrictionKnown: false };

    it("refuses another profile's download rather than assume no parental cap", () => {
      const e = entry({ downloadedByProfileId: "adult-1", downloadedByIsKids: false });
      expect(canPlayOffline(e, unknown, NOW)).toEqual({ allowed: false, reason: "more-restricted-profile" });
    });

    it("still lets the viewer replay a download made by this very profile", () => {
      const e = entry({ downloadedByProfileId: "kid-1" });
      expect(canPlayOffline(e, unknown, NOW)).toEqual({ allowed: true });
    });

    it("leaves legacy entries with no recorded downloader alone", () => {
      expect(canPlayOffline(entry(), unknown, NOW)).toEqual({ allowed: true });
    });

    it("does not tighten anything once the profile HAS been resolved", () => {
      const e = entry({ downloadedByProfileId: "adult-1", downloadedByIsKids: false });
      const resolvedAdult: OfflineProfileInfo = { id: "adult-2", isKids: false, restrictionKnown: true };
      expect(canPlayOffline(e, resolvedAdult, NOW)).toEqual({ allowed: true });
    });
  });

  it("does not retroactively block a legacy entry with no recorded downloader profile", () => {
    const e = entry(); // predates downloadedByProfileId
    expect(canPlayOffline(e, kid10, NOW)).toEqual({ allowed: true });
  });

  it("refuses an entry past the offline TTL", () => {
    const downloadedAt = new Date(NOW - OFFLINE_DOWNLOAD_TTL_MS - 1000).toISOString();
    const e = entry({ downloadedByProfileId: "adult-1", downloadedAt });
    expect(canPlayOffline(e, adult, NOW)).toEqual({ allowed: false, reason: "expired" });
  });

  it("allows an entry right at the edge of the TTL", () => {
    const downloadedAt = new Date(NOW - OFFLINE_DOWNLOAD_TTL_MS + 1000).toISOString();
    const e = entry({ downloadedByProfileId: "adult-1", downloadedAt });
    expect(canPlayOffline(e, adult, NOW)).toEqual({ allowed: true });
  });

  it("does not treat a clock set backward (now before downloadedAt) as expired", () => {
    const downloadedAt = new Date(NOW + 60_000).toISOString();
    const e = entry({ downloadedByProfileId: "adult-1", downloadedAt });
    expect(canPlayOffline(e, adult, NOW)).toEqual({ allowed: true });
  });

  it("fails closed when there is no active profile at all", () => {
    const e = entry({ downloadedByProfileId: "adult-1" });
    expect(canPlayOffline(e, null, NOW)).toEqual({ allowed: false, reason: "unknown-profile" });
    expect(canPlayOffline(e, undefined, NOW)).toEqual({ allowed: false, reason: "unknown-profile" });
    expect(canPlayOffline(e, { id: null }, NOW)).toEqual({ allowed: false, reason: "unknown-profile" });
  });

  it("expiration takes priority over a missing active profile", () => {
    const downloadedAt = new Date(NOW - OFFLINE_DOWNLOAD_TTL_MS - 1000).toISOString();
    const e = entry({ downloadedAt });
    expect(canPlayOffline(e, null, NOW)).toEqual({ allowed: false, reason: "expired" });
  });

  it("uses the profile's region to resolve country-specific certifications over the bare officialRating", () => {
    const e = entry({
      downloadedByProfileId: kid10.id,
      downloadedByIsKids: true,
      downloadedByMaxAge: 10,
      officialRating: "PG-13", // US min age 13 - would exceed the cap of 10
      certificationsByCountry: { US: "PG" }, // US min age 10 - within the cap of 10
    });
    expect(canPlayOffline(e, kid10, NOW)).toEqual({ allowed: true });
  });
});
