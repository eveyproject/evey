import { describe, it, expect } from "vitest";
import { vi } from "vitest";

vi.mock("@tanstack/react-query", () => ({ useQuery: vi.fn() }));
vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
  useUserServiceGetCurrentUser: vi.fn(() => ({ data: undefined })),
  useUserServiceListProfiles: vi.fn(() => ({ data: undefined })),
}));
vi.mock("@evey/store", () => ({ useAuthStore: vi.fn() }));

import { isAgeExceeded, isProfileUnlocked, markUnlocked, pinRequiredFor } from "./useParentalGate";

describe("isAgeExceeded", () => {
  // Never stricter than the server: it fails closed itself (worst-case age
  // across 26 countries, item hidden when it exceeds the cap), so anything the
  // client can see is already vetted. Gating here on a rating this smaller
  // table cannot read would demand a PIN for legitimate all-ages content -
  // e.g. a title whose officialRating is empty but whose FR certification is TP.
  it("defers to the server when the item carries no officialRating", () => {
    expect(isAgeExceeded("US", undefined, 10)).toBe(false);
  });
  it("returns false when maxAge < 0 (profile carries no age cap)", () => {
    expect(isAgeExceeded("US", "R", -1)).toBe(false);
  });
  it("defers to the server when the rating cannot be resolved to an age", () => {
    expect(isAgeExceeded("US", "UNKNOWN", 10)).toBe(false);
  });
  it("still gates a rating it CAN read above the cap", () => {
    expect(isAgeExceeded("US", "R", 12)).toBe(true);
  });
  it("US PG-13 exceeds age 10", () => {
    expect(isAgeExceeded("US", "PG-13", 10)).toBe(true);
  });
  it("US PG-13 allowed at 13", () => {
    expect(isAgeExceeded("US", "PG-13", 13)).toBe(false);
  });
  it("FR -12 exceeds age 10", () => {
    expect(isAgeExceeded("FR", "-12", 10)).toBe(true);
  });
  it("DE 16 exceeds age 12", () => {
    expect(isAgeExceeded("DE", "16", 12)).toBe(true);
  });
  it("falls back to US mapping when country unknown", () => {
    expect(isAgeExceeded("SN", "R", 10)).toBe(true);
  });
});

// pinRequiredFor is the ONE decision every play path shares (Play button,
// episode row, shuffle). Callers whose rating varies per pick (shuffle) call
// it directly instead of the hook's officialRating closure.
describe("pinRequiredFor", () => {
  const gated = {
    profileId: "profile-locked",
    parental: { enabled: true, hasPin: true, maxAge: 12 },
    region: "US",
  };
  it("requires a PIN for a rating above the cap", () => {
    expect(pinRequiredFor({ ...gated, rating: "R" })).toBe(true);
  });
  it("does not gate a rating within the cap", () => {
    expect(pinRequiredFor({ ...gated, rating: "PG" })).toBe(false);
  });
  it("never gates without an active profile", () => {
    expect(pinRequiredFor({ ...gated, profileId: null, rating: "R" })).toBe(false);
  });
  it("never gates when parental controls are disabled or PIN-less", () => {
    expect(pinRequiredFor({ ...gated, parental: { enabled: false, hasPin: true, maxAge: 12 }, rating: "R" })).toBe(false);
    expect(pinRequiredFor({ ...gated, parental: { enabled: true, hasPin: false, maxAge: 12 }, rating: "R" })).toBe(false);
    expect(pinRequiredFor({ ...gated, parental: undefined, rating: "R" })).toBe(false);
  });
  it("skips the prompt once the profile is unlocked for the session", () => {
    markUnlocked("profile-unlocked-1");
    expect(pinRequiredFor({ ...gated, profileId: "profile-unlocked-1", rating: "R" })).toBe(false);
  });
  it("prefers the profile region's own certification over officialRating", () => {
    expect(
      pinRequiredFor({ ...gated, region: "FR", rating: "PG", certificationsByCountry: { FR: "-16" } }),
    ).toBe(true);
  });
});

describe("isProfileUnlocked / markUnlocked", () => {
  it("returns false for unknown profile", () => {
    expect(isProfileUnlocked("profile-xyz-unknown")).toBe(false);
  });
  it("returns true after marking unlocked", () => {
    markUnlocked("profile-abc");
    expect(isProfileUnlocked("profile-abc")).toBe(true);
  });
});
