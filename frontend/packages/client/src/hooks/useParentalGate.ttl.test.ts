import { describe, it, expect, afterEach } from "vitest";
import { vi } from "vitest";

vi.mock("@tanstack/react-query", () => ({ useQuery: vi.fn() }));
vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
  useUserServiceGetCurrentUser: vi.fn(() => ({ data: undefined })),
  useUserServiceListProfiles: vi.fn(() => ({ data: undefined })),
}));
vi.mock("@evey/store", () => ({ useAuthStore: vi.fn() }));

import { isAgeExceeded, isProfileUnlocked, markUnlocked, clearUnlocked, configureParentalUnlock } from "./useParentalGate";

// The television policy: an unlock lasts half an hour (see apps/tv/src/hooks/useParentalGate.ts).
configureParentalUnlock({ ttlMs: 30 * 60 * 1000 });

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

describe("isProfileUnlocked / markUnlocked", () => {
  afterEach(() => {
    vi.useRealTimers();
    clearUnlocked();
  });

  it("returns false for unknown profile", () => {
    expect(isProfileUnlocked("profile-xyz-unknown")).toBe(false);
  });
  it("returns true after marking unlocked", () => {
    markUnlocked("profile-abc");
    expect(isProfileUnlocked("profile-abc")).toBe(true);
  });
  it("expires the unlock after its TTL so a living-room TV re-locks itself", () => {
    vi.useFakeTimers();
    markUnlocked("profile-ttl");
    expect(isProfileUnlocked("profile-ttl")).toBe(true);
    vi.advanceTimersByTime(30 * 60 * 1000 + 1);
    expect(isProfileUnlocked("profile-ttl")).toBe(false);
  });
  it("clearUnlocked drops every unlock so the next viewer starts locked", () => {
    markUnlocked("profile-a");
    markUnlocked("profile-b");
    clearUnlocked();
    expect(isProfileUnlocked("profile-a")).toBe(false);
    expect(isProfileUnlocked("profile-b")).toBe(false);
  });
});
