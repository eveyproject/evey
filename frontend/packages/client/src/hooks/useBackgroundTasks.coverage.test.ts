import { describe, it, expect } from "vitest";
import type { TFunction } from "i18next";
import { coverageLine, coverageProgress, formatEta, overallCoverage, type BackgroundTaskView } from "./useBackgroundTasks";

const t = ((k: string, o?: Record<string, unknown>) => (o ? `${k}:${JSON.stringify(o)}` : k)) as unknown as TFunction;

function task(name: string, coverage?: BackgroundTaskView["coverage"]): BackgroundTaskView {
  return { name, state: "idle", processed: 0, controllable: true, paused: false, processes: 0, rssBytes: 0, cpuPercent: 0, cpuSecondsTotal: 0, forks: 0, coverage };
}

describe("coverageProgress", () => {
  it("weighs by media duration when known: one 3 h film left outweighs nine short episodes done", () => {
    const p = coverageProgress({ doneItems: 9, totalItems: 10, doneSeconds: 9 * 1200, totalSeconds: 9 * 1200 + 10800 })!;
    expect(p.ratio).toBeCloseTo(0.5);
    expect(p.remainingItems).toBe(1);
  });

  it("never reads 100 % while an item is left, and drops the ETA once done", () => {
    const p = coverageProgress({ doneItems: 999, totalItems: 1000, doneSeconds: 999_999, totalSeconds: 1_000_000, etaSeconds: 30 })!;
    expect(coverageLine(t, p)).toContain('"percent":99');
    const done = coverageProgress({ doneItems: 5, totalItems: 5, doneSeconds: 50, totalSeconds: 50, etaSeconds: 30 })!;
    expect(done.complete).toBe(true);
    expect(done.etaSeconds).toBeNull();
    expect(coverageLine(t, done)).toBe("admin.backgroundTasks.upToDate");
  });

  it("is null for a task that counts nothing", () => {
    expect(coverageProgress(undefined)).toBeNull();
    expect(coverageProgress({ doneItems: 0, totalItems: 0, doneSeconds: 0, totalSeconds: 0 })).toBeNull();
  });
});

describe("overallCoverage", () => {
  it("sums the duration of every counted task and takes the slowest ETA", () => {
    const o = overallCoverage([
      task("a", { doneItems: 1, totalItems: 2, doneSeconds: 100, totalSeconds: 200, etaSeconds: 600 }),
      task("b", { doneItems: 3, totalItems: 4, doneSeconds: 300, totalSeconds: 400, etaSeconds: 3600 }),
      task("c", { doneItems: 5, totalItems: 5, doneSeconds: 500, totalSeconds: 500 }),
      task("uncounted"),
    ])!;
    expect(o.ratio).toBeCloseTo(900 / 1100);
    expect(o.remainingItems).toBe(2);
    expect(o.etaSeconds).toBe(3600);
  });

  it("has no ETA while one unfinished task has none yet", () => {
    const o = overallCoverage([
      task("a", { doneItems: 1, totalItems: 2, doneSeconds: 100, totalSeconds: 200, etaSeconds: 600 }),
      task("b", { doneItems: 0, totalItems: 4, doneSeconds: 0, totalSeconds: 400 }),
    ])!;
    expect(o.etaSeconds).toBeNull();
  });

  it("is null when no task counts", () => {
    expect(overallCoverage([task("a")])).toBeNull();
  });
});

describe("formatEta", () => {
  it("rounds to a unit a person reads", () => {
    expect(formatEta(t, 30)).toBe("admin.backgroundTasks.etaUnderMinute");
    expect(formatEta(t, 45 * 60)).toBe('admin.backgroundTasks.etaMinutes:{"count":45}');
    expect(formatEta(t, 89 * 60)).toBe('admin.backgroundTasks.etaMinutes:{"count":89}');
    expect(formatEta(t, 5 * 3600)).toBe('admin.backgroundTasks.etaHours:{"count":5}');
    expect(formatEta(t, 3 * 86400)).toBe('admin.backgroundTasks.etaDays:{"count":3}');
  });
});
