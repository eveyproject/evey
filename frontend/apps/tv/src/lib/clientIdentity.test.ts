import { describe, it, expect, vi } from "vitest";

let platformConstants: Record<string, string> | undefined = { Release: "12", Model: "MiTV-AESP0" };

vi.mock("react-native", () => ({
  get Platform() {
    return { OS: "android", Version: 31, constants: platformConstants };
  },
}));

import { eveyClientUserAgent } from "./clientIdentity";
import appConfig from "../../app.json";

const v = appConfig.expo.version;

describe("eveyClientUserAgent (tv)", () => {
  it("carries the build, like the phone", () => {
    expect(v).toMatch(/^\d+\.\d+\.\d+/);
  });

  it("builds an EveyTV UA with the build, the Android release and the device model", () => {
    platformConstants = { Release: "12", Model: "MiTV-AESP0" };
    expect(eveyClientUserAgent()).toBe(`EveyTV/${v} (Android TV 12; MiTV-AESP0)`);
  });

  it("degrades gracefully without platform constants", () => {
    platformConstants = undefined;
    expect(eveyClientUserAgent()).toBe(`EveyTV/${v} (Android TV)`);
  });
});
