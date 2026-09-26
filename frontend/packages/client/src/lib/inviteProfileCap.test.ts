import { describe, it, expect } from "vitest";
import { parseInviteProfileCap } from "./inviteProfileCap";

describe("parseInviteProfileCap", () => {
  it("reads a whole number above 0 as the cap", () => {
    expect(parseInviteProfileCap("3")).toBe(3);
    expect(parseInviteProfileCap(" 12 ")).toBe(12);
  });

  it("reads empty, 0 or garbage as no cap", () => {
    expect(parseInviteProfileCap("")).toBeUndefined();
    expect(parseInviteProfileCap("0")).toBeUndefined();
    expect(parseInviteProfileCap("-2")).toBeUndefined();
    expect(parseInviteProfileCap("2.5")).toBeUndefined();
    expect(parseInviteProfileCap("abc")).toBeUndefined();
  });
});
