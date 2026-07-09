import { describe, it, expect } from "vitest";
import { vi } from "vitest";

vi.mock("@tanstack/react-query", () => ({ useQuery: vi.fn() }));
vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
  useUserServiceGetCurrentUser: vi.fn(() => ({ data: undefined })),
  useUserServiceListProfiles: vi.fn(() => ({ data: undefined })),
}));
vi.mock("@evey/store", () => ({ useAuthStore: vi.fn() }));

import { isAgeExceeded, isProfileUnlocked, markUnlocked } from "./useParentalGate";

describe("isAgeExceeded", () => {
  it("returns false when rating undefined", () => {
    expect(isAgeExceeded("US", undefined, 10)).toBe(false);
  });
  it("returns false when maxAge < 0", () => {
    expect(isAgeExceeded("US", "R", -1)).toBe(false);
  });
  it("returns false when rating is unknown", () => {
    expect(isAgeExceeded("US", "UNKNOWN", 10)).toBe(false);
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
  it("returns false for unknown profile", () => {
    expect(isProfileUnlocked("profile-xyz-unknown")).toBe(false);
  });
  it("returns true after marking unlocked", () => {
    markUnlocked("profile-abc");
    expect(isProfileUnlocked("profile-abc")).toBe(true);
  });
});
