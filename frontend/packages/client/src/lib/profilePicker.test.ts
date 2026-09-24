import { describe, expect, it } from "vitest";
import { walksThroughProfilePicker } from "./profilePicker";

describe("walksThroughProfilePicker", () => {
  it("walks through a household with nothing to protect", () => {
    expect(walksThroughProfilePicker({ asked: false, profileCount: 1, hasKidsProfile: false })).toBe(true);
    expect(walksThroughProfilePicker({ asked: false, profileCount: 2, hasKidsProfile: false })).toBe(true);
  });

  it("stops where someone could pick wrong", () => {
    expect(walksThroughProfilePicker({ asked: false, profileCount: 2, hasKidsProfile: true })).toBe(false);
  });

  // Two adults and no kids profile: walking through made switching impossible.
  it("always shows the picker when the viewer asked to switch", () => {
    expect(walksThroughProfilePicker({ asked: true, profileCount: 2, hasKidsProfile: false })).toBe(false);
    expect(walksThroughProfilePicker({ asked: true, profileCount: 1, hasKidsProfile: false })).toBe(false);
  });
});
