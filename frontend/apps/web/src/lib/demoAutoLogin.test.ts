import { describe, it, expect, beforeEach } from "vitest";
import { demoAutoLoginAllowed, markDemoManualLogout, MANUAL_LOGOUT_HOLD_MS } from "./demoAutoLogin";

beforeEach(() => {
  localStorage.clear();
});

// A visitor who never signed out is signed in again; one who did is left on
// the form for a day, then signed in again.
describe("demo auto-login memory", () => {
  it("allows by default", () => {
    expect(demoAutoLoginAllowed()).toBe(true);
  });

  it("holds the form for a day after a deliberate sign-out, not longer", () => {
    const t0 = 1_700_000_000_000;
    markDemoManualLogout(t0);
    expect(demoAutoLoginAllowed(t0 + 1000)).toBe(false);
    expect(demoAutoLoginAllowed(t0 + MANUAL_LOGOUT_HOLD_MS - 1)).toBe(false);
    expect(demoAutoLoginAllowed(t0 + MANUAL_LOGOUT_HOLD_MS)).toBe(true);
  });

  it("treats a corrupt value as no sign-out", () => {
    localStorage.setItem("evey-demo-manual-logout-at", "yesterday");
    expect(demoAutoLoginAllowed()).toBe(true);
  });
});
