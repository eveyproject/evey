import { describe, it, expect } from "vitest";
import { newerClientRelease } from "./appUpdate";

describe("newerClientRelease", () => {
  const health = (clients: unknown) => ({ status: "ok", clients });

  it("offers each app its own newer build and nothing for the installed or an older one", () => {
    const h = health({ tv: { version: "0.1.21", version_code: 86 }, mobile: { version: "0.0.1-beta.79", version_code: 85 } });
    expect(newerClientRelease(h, "mobile", 76)).toEqual({ version: "0.0.1-beta.79", versionCode: 85 });
    expect(newerClientRelease(h, "tv", 84)).toEqual({ version: "0.1.21", versionCode: 86 });
    expect(newerClientRelease(h, "mobile", 85)).toBeNull();
    expect(newerClientRelease(h, "tv", 90)).toBeNull();
  });

  // Mobile and TV share one versionCode sequence: a phone on 85 must not be
  // told to update because the TV is on 86.
  it("never reads the other app's key", () => {
    const tvOnly = health({ tv: { version: "0.1.21", version_code: 86 } });
    expect(newerClientRelease(tvOnly, "mobile", 76)).toBeNull();
    expect(newerClientRelease(tvOnly, "tv", 76)).not.toBeNull();
  });

  it("offers nothing when an older or unstamped server says nothing, or the field is malformed", () => {
    expect(newerClientRelease({ status: "ok" }, "mobile", 1)).toBeNull();
    expect(newerClientRelease(null, "tv", 1)).toBeNull();
    expect(newerClientRelease(health({ mobile: { version: "x", version_code: "85" } }), "mobile", 1)).toBeNull();
    expect(newerClientRelease(health({ mobile: { version: "x", version_code: 85.5 } }), "mobile", 1)).toBeNull();
  });

  it("names the build by its code when the version string is missing", () => {
    expect(newerClientRelease(health({ mobile: { version_code: 85 } }), "mobile", 1)).toEqual({ version: "85", versionCode: 85 });
  });
});
