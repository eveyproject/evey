import { describe, it, expect } from "vitest";
import { onResultPicked, onScanStarted, rowClaimsFocus, scanFocusIdle } from "./scanFocus";

describe("scan focus handoffs", () => {
  it("idle: nothing claims the focus (a stale result list must not steal it)", () => {
    expect(rowClaimsFocus(scanFocusIdle, 0)).toBe(false);
    expect(rowClaimsFocus(scanFocusIdle, 3)).toBe(false);
  });

  it("a scan arms the handoff: the FIRST row that arrives claims it, no other", () => {
    const armed = onScanStarted();
    expect(rowClaimsFocus(armed, 0)).toBe(true);
    expect(rowClaimsFocus(armed, 1)).toBe(false);
    expect(rowClaimsFocus(armed, 7)).toBe(false);
  });

  it("picking a result spends the handoff - the URL field has the focus now", () => {
    const after = onResultPicked();
    expect(after.focusFirstResult).toBe(false);
    expect(rowClaimsFocus(after, 0)).toBe(false);
  });

  it("a second scan re-arms it", () => {
    expect(rowClaimsFocus(onScanStarted(), 0)).toBe(true);
  });
});
