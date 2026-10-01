import { describe, it, expect } from "vitest";
import { nextRefreshDelay, REFRESH_MARGIN_MS } from "./tokenRefreshDelay";

describe("nextRefreshDelay", () => {
  const now = 1_000_000;

  it("fires the margin before expiry while refreshes succeed", () => {
    expect(nextRefreshDelay(now + 10 * 60_000, now, 0, 0)).toBe(10 * 60_000 - REFRESH_MARGIN_MS);
  });

  it("a token past its refresh time is refreshed at once the first time", () => {
    expect(nextRefreshDelay(now + 30_000, now, 0, 0)).toBe(0);
  });

  it("never retries a failed refresh with a zero delay, and backs off to a minute", () => {
    const delays = [1, 2, 3, 4, 5, 6, 7, 10].map((f) => nextRefreshDelay(now - 5_000, now, 0, f));
    expect(delays).toEqual([1000, 2000, 4000, 8000, 16000, 32000, 60000, 60000]);
  });

  it("a failure never brings a far refresh closer", () => {
    expect(nextRefreshDelay(now + 10 * 60_000, now, 0, 3)).toBe(10 * 60_000 - REFRESH_MARGIN_MS);
  });
});
