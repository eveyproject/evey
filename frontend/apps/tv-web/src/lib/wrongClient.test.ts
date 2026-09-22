import { describe, expect, it } from "vitest";
import { asksToStay, shouldLeaveTvClient } from "./wrongClient";

const base = { protocol: "https:", hostname: "evey.example.com", search: "", forced: false };

const TELEVISIONS = [
  "Mozilla/5.0 (Web0S; Linux/SmartTV) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/108.0.0.0 Safari/537.36 WebAppManager",
  "Mozilla/5.0 (SMART-TV; LINUX; Tizen 7.0) AppleWebKit/537.36 Chrome/94.0.4606.31 TV Safari/537.36",
  "Mozilla/5.0 (Linux; Android 11.0; Build/CALIFORNIE) AppleWebKit/537.36 Chrome/92.0.4515.0 Safari/537.36 CrKey/1.56.500000 DeviceType/AndroidTV",
  "Mozilla/5.0 (Linux; Android 9; AFTKA Build/PS7233) AppleWebKit/537.36 Chrome/70.0.3538.110 Safari/537.36",
  "Mozilla/5.0 (VIDAA; Linux) AppleWebKit/537.36 Chrome/79.0 Safari/537.36",
];

const NOT_TELEVISIONS = [
  "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/128.0.0.0 Safari/537.36",
  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 Version/17.0 Safari/605.1.15",
  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/127.0 Safari/537.36",
  "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Version/17.0 Mobile/15E148 Safari/604.1",
];

describe("the television client knows when it is the wrong application", () => {
  it.each(TELEVISIONS)("stays put on a television", (userAgent) => {
    expect(shouldLeaveTvClient({ ...base, userAgent })).toBe(false);
  });

  it.each(NOT_TELEVISIONS)("hands a desktop or a phone back to the web application", (userAgent) => {
    expect(shouldLeaveTvClient({ ...base, userAgent })).toBe(true);
  });

  // The packaged application has no web application next to it to go to.
  it("never redirects the packaged application", () => {
    expect(shouldLeaveTvClient({ ...base, protocol: "file:", userAgent: NOT_TELEVISIONS[0] })).toBe(false);
  });

  it("leaves a development machine alone", () => {
    for (const hostname of ["localhost", "127.0.0.1", "192.168.1.42", "10.0.0.7", "evey.local"]) {
      expect(shouldLeaveTvClient({ ...base, hostname, userAgent: NOT_TELEVISIONS[0] })).toBe(false);
    }
  });

  it("obeys a viewer who asked to stay, in the address or from a previous visit", () => {
    expect(shouldLeaveTvClient({ ...base, search: "?tv=1", userAgent: NOT_TELEVISIONS[0] })).toBe(false);
    expect(shouldLeaveTvClient({ ...base, search: "?a=b&tv=1", userAgent: NOT_TELEVISIONS[0] })).toBe(false);
    expect(shouldLeaveTvClient({ ...base, forced: true, userAgent: NOT_TELEVISIONS[0] })).toBe(false);
  });

  it("reads the request to stay out of the address", () => {
    expect(asksToStay("?tv=1")).toBe(true);
    expect(asksToStay("?tv=10")).toBe(false);
    expect(asksToStay("")).toBe(false);
  });
});
