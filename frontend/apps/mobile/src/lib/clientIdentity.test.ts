import { describe, it, expect } from "vitest";
import { vi } from "vitest";

// The mobile app's API requests used to reach the server as "okhttp/4.12.0":
// unreadable in the admin logs and rendered with a desktop icon on the online
// panel. The app now identifies itself with a product UA the panel and the
// future TV app (EveyTV) share the shape of.
let platformOS = "android";
let platformVersion: number | string = 34;
let platformConstants: Record<string, string> | undefined = { Release: "14", Model: "SM-G991B" };

vi.mock("react-native", () => ({
  get Platform() {
    return { OS: platformOS, Version: platformVersion, constants: platformConstants };
  },
}));

import { eveyClientUserAgent } from "./clientIdentity";

describe("eveyClientUserAgent", () => {
  it("builds an EveyMobile UA with Android release and device model", () => {
    platformOS = "android";
    platformConstants = { Release: "14", Model: "SM-G991B" };
    expect(eveyClientUserAgent()).toBe("EveyMobile (Android 14; SM-G991B)");
  });

  it("degrades gracefully when the platform constants are missing", () => {
    platformOS = "android";
    platformConstants = undefined;
    expect(eveyClientUserAgent()).toBe("EveyMobile (Android)");
  });

  it("labels iOS with its version", () => {
    platformOS = "ios";
    platformVersion = "17.4";
    expect(eveyClientUserAgent()).toBe("EveyMobile (iOS 17.4)");
  });
});
