import { describe, it, expect } from "vitest";
import { vi } from "vitest";

vi.mock("@tanstack/react-query", () => ({ useQuery: vi.fn() }));
vi.mock("@evey/api", () => ({ eveyFetch: vi.fn() }));
vi.mock("@evey/store", () => ({ useAuthStore: vi.fn() }));

import { isRatingExceeded, isProfileUnlocked, markUnlocked } from "./useParentalGate";

describe("isRatingExceeded", () => {
  it("returns false when contentRating is undefined", () => {
    expect(isRatingExceeded(undefined, "PG")).toBe(false);
  });

  it("returns false when maxRating is empty string", () => {
    expect(isRatingExceeded("R", "")).toBe(false);
  });

  it("returns false when contentRating is unknown", () => {
    expect(isRatingExceeded("UNKNOWN", "PG")).toBe(false);
  });

  it("returns false when maxRating is unknown", () => {
    expect(isRatingExceeded("PG", "UNKNOWN")).toBe(false);
  });

  it("returns false when content equals max", () => {
    expect(isRatingExceeded("PG", "PG")).toBe(false);
  });

  it("returns false when content is below max", () => {
    expect(isRatingExceeded("G", "PG")).toBe(false);
    expect(isRatingExceeded("PG", "R")).toBe(false);
    expect(isRatingExceeded("G", "NC-17")).toBe(false);
  });

  it("returns true when content exceeds max", () => {
    expect(isRatingExceeded("R", "PG")).toBe(true);
    expect(isRatingExceeded("PG-13", "G")).toBe(true);
    expect(isRatingExceeded("NC-17", "R")).toBe(true);
  });

  it("handles TV-Y as equivalent to G (severity 0)", () => {
    expect(isRatingExceeded("TV-Y", "G")).toBe(false);
    expect(isRatingExceeded("G", "TV-Y")).toBe(false);
  });

  it("handles TV-PG as equivalent to PG (severity 1)", () => {
    expect(isRatingExceeded("TV-PG", "PG")).toBe(false);
    expect(isRatingExceeded("PG", "TV-PG")).toBe(false);
  });

  it("handles TV-14 as equivalent to PG-13 (severity 2)", () => {
    expect(isRatingExceeded("TV-14", "PG-13")).toBe(false);
    expect(isRatingExceeded("TV-14", "PG")).toBe(true);
  });

  it("handles TV-MA as equivalent to R (severity 3)", () => {
    expect(isRatingExceeded("TV-MA", "R")).toBe(false);
    expect(isRatingExceeded("TV-MA", "PG-13")).toBe(true);
  });

  it("is case-insensitive", () => {
    expect(isRatingExceeded("r", "pg")).toBe(true);
    expect(isRatingExceeded("  R  ", "  PG  ")).toBe(true);
  });

  it("NC-17 exceeds all other ratings", () => {
    expect(isRatingExceeded("NC-17", "R")).toBe(true);
    expect(isRatingExceeded("NC-17", "PG-13")).toBe(true);
    expect(isRatingExceeded("NC-17", "PG")).toBe(true);
    expect(isRatingExceeded("NC-17", "G")).toBe(true);
  });

  it("G is not exceeded by anything lower", () => {
    expect(isRatingExceeded("G", "G")).toBe(false);
    expect(isRatingExceeded("TV-Y", "G")).toBe(false);
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
