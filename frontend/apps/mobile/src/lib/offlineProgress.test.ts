// The rule these tests pin: everything a viewer watches offline is recorded
// ONLY here (no play session exists without the server), so this store has to
// survive a kill, and its record has to be able to beat a stale server one -
// but only when it is genuinely fresher, never merely bigger.
import { describe, it, expect, vi, beforeEach } from "vitest";

const files = new Map<string, string>();
vi.mock("expo-file-system/legacy", () => ({
  documentDirectory: "/data/user/0/app/files/",
  getInfoAsync: vi.fn(async () => ({ exists: true })),
  makeDirectoryAsync: vi.fn(async () => undefined),
  readAsStringAsync: vi.fn(async (path: string) => {
    const v = files.get(String(path));
    if (v === undefined) throw new Error("not found");
    // Yield, so two writers get a chance to interleave on the read.
    await new Promise((r) => setTimeout(r, 0));
    return v;
  }),
  writeAsStringAsync: vi.fn(async (path: string, data: string) => {
    files.set(String(path), data);
  }),
  moveAsync: vi.fn(async ({ from, to }: { from: string; to: string }) => {
    const v = files.get(String(from));
    if (v !== undefined) {
      files.set(String(to), v);
      files.delete(String(from));
    }
  }),
}));

import {
  saveOfflineProgress,
  getOfflineProgress,
  clearOfflineProgress,
  preferLocalProgress,
} from "./offlineProgress";

const FILE = "/data/user/0/app/files/downloads/progress.json";

beforeEach(() => {
  files.clear();
});

describe("offline playhead store", () => {
  it("round-trips a position and its duration", async () => {
    await saveOfflineProgress("m1", 2_400_000, 7_200_000);
    const got = await getOfflineProgress("m1");
    expect(got?.positionMs).toBe(2_400_000);
    expect(got?.durationMs).toBe(7_200_000);
    expect(Date.parse(got!.updatedAt)).toBeGreaterThan(0);
  });

  it("replaces the previous record for the same title instead of stacking rows", async () => {
    await saveOfflineProgress("m1", 1000, 7_200_000);
    await saveOfflineProgress("m1", 5000, 7_200_000);
    expect(JSON.parse(files.get(FILE)!)).toHaveLength(1);
    expect((await getOfflineProgress("m1"))?.positionMs).toBe(5000);
  });

  it("answers nothing for a title it has never seen", async () => {
    expect(await getOfflineProgress("unknown")).toBeNull();
  });

  it("keeps both titles when two saves overlap", async () => {
    await Promise.all([
      saveOfflineProgress("m1", 1000, 10),
      saveOfflineProgress("m2", 2000, 20),
    ]);
    expect(JSON.parse(files.get(FILE)!)).toHaveLength(2);
  });

  it("clears one title without touching the others", async () => {
    await saveOfflineProgress("m1", 1000, 10);
    await saveOfflineProgress("m2", 2000, 20);
    await clearOfflineProgress("m1");
    expect(await getOfflineProgress("m1")).toBeNull();
    expect((await getOfflineProgress("m2"))?.positionMs).toBe(2000);
  });

  it("never writes the live file directly, so a reader cannot see half of it", async () => {
    await saveOfflineProgress("m1", 1000, 10);
    const fs = await import("expo-file-system/legacy");
    for (const [path] of (fs.writeAsStringAsync as unknown as { mock: { calls: string[][] } }).mock.calls) {
      expect(String(path)).toMatch(/progress\.json\.tmp$/);
    }
  });

  it("refuses a nonsense position rather than storing it", async () => {
    await saveOfflineProgress("m1", -5, 10);
    await saveOfflineProgress("m2", Number.NaN, 10);
    expect(files.get(FILE)).toBeUndefined();
  });
});

describe("preferLocalProgress", () => {
  const t = (iso: string) => iso;

  it("prefers the local record when the server has never heard of the title", () => {
    expect(preferLocalProgress({ positionMs: 5000, updatedAt: t("2026-09-06T10:00:00Z") }, null)).toBe(true);
  });

  it("prefers the local record when it is more recent", () => {
    expect(
      preferLocalProgress(
        { positionMs: 5000, updatedAt: t("2026-09-06T12:00:00Z") },
        t("2026-09-06T08:00:00Z"),
      ),
    ).toBe(true);
  });

  // The case a "bigger position wins" rule gets wrong: watched further on the
  // web after the flight, the server is the one to believe.
  it("defers to the server when the server record is more recent", () => {
    expect(
      preferLocalProgress(
        { positionMs: 5_000_000, updatedAt: t("2026-09-06T08:00:00Z") },
        t("2026-09-06T12:00:00Z"),
      ),
    ).toBe(false);
  });

  // The other case it gets wrong: restarted from the beginning offline.
  it("still prefers a fresher local record even when its position is smaller", () => {
    expect(
      preferLocalProgress(
        { positionMs: 1000, updatedAt: t("2026-09-06T12:00:00Z") },
        t("2026-09-06T08:00:00Z"),
      ),
    ).toBe(true);
  });

  it("ignores a local record at position zero", () => {
    expect(preferLocalProgress({ positionMs: 0, updatedAt: t("2026-09-06T12:00:00Z") }, null)).toBe(false);
  });

  it("ignores an absent local record", () => {
    expect(preferLocalProgress(null, t("2026-09-06T12:00:00Z"))).toBe(false);
  });

  it("does not let an undated local record outrank a dated server one", () => {
    expect(preferLocalProgress({ positionMs: 5000 }, t("2026-09-06T12:00:00Z"))).toBe(false);
  });
});
