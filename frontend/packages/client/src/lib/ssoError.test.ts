import { describe, it, expect } from "vitest";
import { ssoErrorKey, SSO_ERROR_REASONS } from "./ssoError";

describe("ssoErrorKey", () => {
  it("maps every server reason to its own message", () => {
    for (const r of SSO_ERROR_REASONS) expect(ssoErrorKey(r)).toBe(`auth.login.ssoError.${r}`);
  });

  it("falls back to the generic failure for anything else", () => {
    expect(ssoErrorKey("failed")).toBe("auth.login.ssoFailed");
    expect(ssoErrorKey("1")).toBe("auth.login.ssoFailed");
    expect(ssoErrorKey(undefined)).toBe("auth.login.ssoFailed");
    expect(ssoErrorKey("constructor")).toBe("auth.login.ssoFailed");
  });
});
