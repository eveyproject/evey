import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { QueryClient, QueryObserver } from "@tanstack/react-query";
import { flushWatchedState, invalidateWatchedState } from "./invalidateWatchedState";

// A query on screen: an observer keeps it active, fetches counts its refetches.
function onScreen(qc: QueryClient, key: string) {
  const counter = { fetches: 0 };
  const observer = new QueryObserver(qc, {
    queryKey: [key],
    queryFn: async () => { counter.fetches++; return { at: counter.fetches }; },
    staleTime: Infinity,
  });
  const unsubscribe = observer.subscribe(() => {});
  return { counter, unsubscribe };
}

async function settle() {
  await vi.advanceTimersByTimeAsync(500);
}

describe("invalidateWatchedState", () => {
  beforeEach(() => { vi.useFakeTimers(); });
  afterEach(() => { vi.useRealTimers(); });

  // Every key, not a chosen list: a key missing from the old list (Continue
  // Watching, Most Popular, the library rails) kept a watched title on screen.
  it("refreshes every query on screen, whatever its key", async () => {
    const qc = new QueryClient();
    const keys = ["/api/v1/media", "/api/v1/feeds/recommended", "/api/v1/playback/resume", "/api/v1/some/future/list"];
    const shown = keys.map((k) => onScreen(qc, k));
    await settle();
    invalidateWatchedState(qc, "mi-1");
    await settle();
    for (const s of shown) expect(s.counter.fetches).toBe(2);
    qc.setQueryData(["/api/v1/off-screen"], { x: 1 });
    invalidateWatchedState(qc);
    await settle();
    expect(qc.getQueryState(["/api/v1/off-screen"])?.isInvalidated).toBe(true);
    shown.forEach((s) => s.unsubscribe());
  });

  // A bulk mark fires one mutation per title: their refreshes share one pass,
  // so fifty marks do not become fifty rounds of every query on screen.
  it("coalesces a burst of marks into one refresh", async () => {
    const qc = new QueryClient();
    const shown = onScreen(qc, "/api/v1/media");
    await settle();
    for (let i = 0; i < 50; i++) invalidateWatchedState(qc, `mi-${i}`);
    await settle();
    expect(shown.counter.fetches).toBe(2);
    shown.unsubscribe();
  });

  // The title a player shows is marked stale, not refetched under it.
  it("leaves the playing title's own page alone until it is shown again", async () => {
    const qc = new QueryClient();
    const playing = onScreen(qc, "/api/v1/media/mi-1");
    const rail = onScreen(qc, "/api/v1/media");
    await settle();
    invalidateWatchedState(qc, "mi-1", { playingId: "mi-1" });
    await settle();
    expect(playing.counter.fetches).toBe(1);
    expect(qc.getQueryState(["/api/v1/media/mi-1"])?.isInvalidated).toBe(true);
    expect(rail.counter.fetches).toBe(2);
    playing.unsubscribe();
    rail.unsubscribe();
  });

  it("flushes on demand", async () => {
    const qc = new QueryClient();
    const shown = onScreen(qc, "/api/v1/media");
    await settle();
    invalidateWatchedState(qc);
    flushWatchedState(qc);
    await vi.advanceTimersByTimeAsync(0);
    expect(shown.counter.fetches).toBe(2);
    shown.unsubscribe();
  });
});
