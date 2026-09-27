import { describe, it, expect, vi } from "vitest";
import { invalidateWatchedState } from "./invalidateWatchedState";

function fakeQC() {
  const calls: unknown[] = [];
  const qc = {
    invalidateQueries: vi.fn((arg: unknown) => { calls.push(arg); }),
  };
  return { qc: qc as never, calls, spy: qc.invalidateQueries };
}

// The predicate must accept every watched-sensitive feed, and CRUCIALLY the
// "Up Next" row (/api/v1/playback/next-up), which was missing from all four
// hand-rolled predicates and left a finished episode stuck for 5 minutes.
describe("invalidateWatchedState", () => {
  function predicateFor(calls: unknown[]) {
    const withPredicate = calls.find(
      (c): c is { predicate: (q: { queryKey: unknown[] }) => boolean } =>
        typeof (c as { predicate?: unknown }).predicate === "function",
    );
    if (!withPredicate) throw new Error("no predicate invalidation issued");
    return (key: string) => withPredicate.predicate({ queryKey: [key] });
  }

  it("invalidates the Up Next row, home feeds and Most Popular", () => {
    const { qc, calls } = fakeQC();
    invalidateWatchedState(qc, "mi-1");
    const matches = predicateFor(calls);
    expect(matches("/api/v1/playback/next-up")).toBe(true);
    expect(matches("/api/v1/feeds/recommended")).toBe(true);
    expect(matches("/api/v1/feeds/resume-playback")).toBe(true);
    expect(matches("/api/v1/popular")).toBe(true);
    expect(matches("/api/v1/libraries")).toBe(false);
  });

  // The Continue Watching row of all four homes reads /api/v1/playback/resume,
  // not a /feeds/ key: a title finished in the player stayed in it for the
  // cache's lifetime, longer on a television whose home tab never unmounts.
  it("invalidates the Continue Watching row", () => {
    const { qc, calls } = fakeQC();
    invalidateWatchedState(qc, "mi-1");
    expect(predicateFor(calls)("/api/v1/playback/resume")).toBe(true);
  });

  it("invalidates the item + its progress when a mediaId is given", () => {
    const { qc, spy } = fakeQC();
    invalidateWatchedState(qc, "mi-1");
    expect(spy).toHaveBeenCalledWith({ queryKey: ["/api/v1/media/mi-1"] });
    expect(spy).toHaveBeenCalledWith({ queryKey: ["/api/v1/playback/mi-1/progress"] });
    expect(spy).toHaveBeenCalledWith({ queryKey: ["/api/v1/playback/history"] });
  });

  it("skips the item keys when no mediaId is given", () => {
    const { qc, spy } = fakeQC();
    invalidateWatchedState(qc);
    const keys = spy.mock.calls.map((c) => (c[0] as { queryKey?: string[] }).queryKey?.[0]).filter(Boolean);
    expect(keys).toContain("/api/v1/playback/history");
    expect(keys).not.toContain("/api/v1/media/undefined");
  });
});
