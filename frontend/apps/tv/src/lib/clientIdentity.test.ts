import { describe, it, expect, vi } from "vitest";

let platformConstants: Record<string, string> | undefined = { Release: "12", Model: "MiTV-AESP0" };

vi.mock("react-native", () => ({
  get Platform() {
    return { OS: "android", Version: 31, constants: platformConstants };
  },
}));

import { eveyClientUserAgent } from "./clientIdentity";

describe("eveyClientUserAgent (tv)", () => {
  it("builds an EveyTV UA with the Android release and device model", () => {
    platformConstants = { Release: "12", Model: "MiTV-AESP0" };
    expect(eveyClientUserAgent()).toBe("EveyTV (Android TV 12; MiTV-AESP0)");
  });

  it("degrades gracefully without platform constants", () => {
    platformConstants = undefined;
    expect(eveyClientUserAgent()).toBe("EveyTV (Android TV)");
  });
});
