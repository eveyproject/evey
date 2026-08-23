import { describe, it, expect } from "vitest";
import { isRemoteAccessDisabled, remoteAccessDisabledMessage } from "./remoteAccessError";

const t = (_k: string, fb: string) => fb;

describe("isRemoteAccessDisabled", () => {
  it("recognizes the errorCode eveyFetch attaches", () => {
    expect(isRemoteAccessDisabled(Object.assign(new Error("nope"), { errorCode: "remote_access_disabled" }))).toBe(true);
  });

  it("recognizes a raw response body with a code field", () => {
    expect(isRemoteAccessDisabled({ code: "remote_access_disabled", message: "remote access disabled" })).toBe(true);
  });

  it("falls back to the message when the code is lost", () => {
    expect(isRemoteAccessDisabled(new Error("remote access disabled"))).toBe(true);
    expect(isRemoteAccessDisabled(new Error("Remote Access Disabled"))).toBe(true);
  });

  it("ignores unrelated errors", () => {
    expect(isRemoteAccessDisabled(new Error("invalid credentials"))).toBe(false);
    expect(isRemoteAccessDisabled(Object.assign(new Error("rate limited"), { errorCode: "rate_limited" }))).toBe(false);
    expect(isRemoteAccessDisabled(null)).toBe(false);
    expect(isRemoteAccessDisabled(undefined)).toBe(false);
    expect(isRemoteAccessDisabled("network request failed")).toBe(false);
  });
});

describe("remoteAccessDisabledMessage", () => {
  it("renders a title and an actionable body", () => {
    expect(remoteAccessDisabledMessage(t)).toEqual({
      title: "Remote access disabled",
      body: "Remote access is disabled on this server. The administrator must enable it in Admin > Remote access to allow connections from outside the local network.",
    });
  });
});
