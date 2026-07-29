import { describe, it, expect } from "vitest";
import { versionLabel } from "./versionLabel";

describe("versionLabel", () => {
  it("prefixes a numeric version", () => {
    expect(versionLabel("1.2.3")).toBe("v1.2.3");
    expect(versionLabel("0.0.1")).toBe("v0.0.1");
  });

  it("leaves a named version alone (this is the 'vdev' report)", () => {
    expect(versionLabel("dev")).toBe("dev");
    expect(versionLabel("beta-0.0.1")).toBe("beta-0.0.1");
    expect(versionLabel("nightly")).toBe("nightly");
  });

  it("renders nothing for an absent version", () => {
    expect(versionLabel(undefined)).toBe("");
    expect(versionLabel(null)).toBe("");
    expect(versionLabel("  ")).toBe("");
  });
});
