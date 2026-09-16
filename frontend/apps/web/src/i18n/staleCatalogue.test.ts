import { describe, it, expect, vi, beforeEach } from "vitest";
import i18n from "i18next";
import { refetchOnMissingKey } from "./index";

// Prod, 2026-09-16: a tab left open across a deploy showed raw keys on a
// freshly loaded admin chunk - the in-memory catalogue predated the bundle.
describe("refetchOnMissingKey", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("refetches the catalogue on a missing key, at most once a minute", () => {
    const reload = vi.spyOn(i18n, "reloadResources").mockResolvedValue(undefined as never);
    // DEV builds never refetch (Vite serves the live catalogue); the test
    // runner is not DEV, so the guard below is the throttle only.
    if (import.meta.env.DEV) return;
    const t0 = 10_000_000;
    refetchOnMissingKey(t0);
    refetchOnMissingKey(t0 + 5_000);
    expect(reload).toHaveBeenCalledTimes(1);
    refetchOnMissingKey(t0 + 61_000);
    expect(reload).toHaveBeenCalledTimes(2);
  });
});
