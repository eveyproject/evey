import { describe, it, expect } from "vitest";
import { authGateDecision } from "./authGate";

// The (auth) layout's gate. Only a REFUSED refresh ends the session; an
// unanswered one is the network, not the credential. And while the phone is
// offline the tree must render at once: the refresh cannot be refused
// there, and holding the app blank until the attempt times out (8 s on a
// silent link) is exactly what keeps a viewer from their downloads.
describe("authGateDecision", () => {
  const base = { accessToken: "tok", expired: false, refreshState: "pending" as const, online: true, profileId: "p1" };

  it("no token: login; no profile: picker; fresh token: render", () => {
    expect(authGateDecision({ ...base, accessToken: null })).toBe("login");
    expect(authGateDecision({ ...base, profileId: null })).toBe("profile-select");
    expect(authGateDecision(base)).toBe("render");
  });

  it("an expired token waits for the refresh online, and only a refusal logs out", () => {
    expect(authGateDecision({ ...base, expired: true, refreshState: "pending" })).toBe("wait");
    expect(authGateDecision({ ...base, expired: true, refreshState: "refused" })).toBe("login");
    expect(authGateDecision({ ...base, expired: true, refreshState: "unavailable" })).toBe("render");
  });

  it("offline, an expired token never holds the app: the tree renders while the refresh is pending", () => {
    expect(authGateDecision({ ...base, expired: true, refreshState: "pending", online: false })).toBe("render");
    expect(authGateDecision({ ...base, expired: true, refreshState: "unavailable", online: false })).toBe("render");
    // A refusal that already landed still counts, offline or not.
    expect(authGateDecision({ ...base, expired: true, refreshState: "refused", online: false })).toBe("login");
    expect(authGateDecision({ ...base, expired: true, online: false, profileId: null })).toBe("profile-select");
  });
});
