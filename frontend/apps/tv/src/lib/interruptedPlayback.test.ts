import { describe, it, expect, vi, beforeEach } from "vitest";

const store = new Map<string, string>();
vi.mock("expo-secure-store", () => ({
  getItemAsync: vi.fn(async (k: string) => store.get(k) ?? null),
  setItemAsync: vi.fn(async (k: string, v: string) => { store.set(k, v); }),
  deleteItemAsync: vi.fn(async (k: string) => { store.delete(k); }),
}));

import { markPlaying, clearPlaying, takeInterrupted, resumeRoute, INTERRUPTED_MAX_AGE_MS, type InterruptedPlayback } from "./interruptedPlayback";

const NOW = 1_760_000_000_000;
const mark = (over: Partial<InterruptedPlayback> = {}): InterruptedPlayback => ({
  mediaId: "m1", profileId: "p1", title: "Film", positionMs: 2_530_000, pick: { audio: 2 }, at: NOW - 60_000, ...over,
});

describe("interrupted playback", () => {
  beforeEach(() => store.clear());

  it("offers a mark left by a player the system killed, once", async () => {
    await markPlaying(mark());
    const got = await takeInterrupted("p1", NOW);
    expect(got).toMatchObject({ mediaId: "m1", title: "Film", positionMs: 2_530_000, pick: { audio: 2 } });
    expect(await takeInterrupted("p1", NOW)).toBeNull();
  });

  it("offers nothing once the player was left normally", async () => {
    await markPlaying(mark());
    await clearPlaying();
    expect(await takeInterrupted("p1", NOW)).toBeNull();
  });

  it("never offers another profile's title, and drops the mark", async () => {
    await markPlaying(mark());
    expect(await takeInterrupted("p2", NOW)).toBeNull();
    expect(await takeInterrupted("p1", NOW)).toBeNull();
  });

  it("lets a stale mark go", async () => {
    await markPlaying(mark({ at: NOW - INTERRUPTED_MAX_AGE_MS - 1 }));
    expect(await takeInterrupted("p1", NOW)).toBeNull();
  });

  it("survives a corrupt mark", async () => {
    store.set("evey.interruptedPlayback", "{not json");
    expect(await takeInterrupted("p1", NOW)).toBeNull();
    expect(store.size).toBe(0);
  });

  it("resumes through the player route with the tracks the viewer had", () => {
    expect(resumeRoute(mark({ pick: { audio: 2, subtitle: "off" } }))).toEqual({
      pathname: "/player/[id]",
      params: { id: "m1", audio: "2", subtitle: "off" },
    });
  });
});
