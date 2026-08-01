import { describe, it, expect, vi, beforeEach } from "vitest";

const state = vi.hoisted(() => ({
  compat: false,
  compatPermille: 0,
  reduced: false,
  collected: 0,
  slices: 0,
  reducedPermille: 0,
}));

vi.mock("./downloads", () => ({
  isPreparingCompat: () => state.compat,
  getCompatPreparePermille: () => state.compatPermille,
}));
vi.mock("./reducedDownloadState", () => ({
  isReducedDownloadRunning: () => state.reduced,
  getReducedPreparePermille: () => state.reducedPermille,
  getReducedSliceProgress: () => ({ collected: state.collected, slices: state.slices }),
}));

import { downloadStageLabel } from "./downloadStageLabel";

const t = (_k: string, d: string, o?: Record<string, unknown>) =>
  d.replace(/\{\{(\w+)\}\}/g, (_, k) => String(o?.[k] ?? ""));
const deps = { formatSize: (b: number) => `${Math.round(b / 1024 ** 2)}MB` };
const label = (progress: number | null, bytes: { downloaded: number; total: number } | null = null, rate = 0) =>
  downloadStageLabel(t, "m1", progress, bytes, rate, deps);

beforeEach(() => {
  Object.assign(state, { compat: false, compatPermille: 0, reduced: false, collected: 0, slices: 0, reducedPermille: 0 });
});

describe("downloadStageLabel", () => {
  it("reports bytes and rate for an ordinary transfer", () => {
    expect(label(0.42, { downloaded: 100 * 1024 ** 2, total: 200 * 1024 ** 2 }, 5 * 1024 ** 2))
      .toBe("42% · 100MB / 200MB · 5MB/s");
  });

  it("never leaves a running download with a blank line", () => {
    expect(label(NaN)).toBe("Downloading...");
  });

  // The percent on a minutes-long server-side stage is not decoration: a
  // static label read as a hang in the field within a day of shipping it.
  it("shows how far the server has got preparing audio", () => {
    state.compat = true;
    expect(label(NaN)).toBe("Preparing audio on the server...");
    state.compatPermille = 376;
    expect(label(NaN)).toBe("Preparing audio on the server... 38%");
  });

  it("shows how far the server has got encoding the 720p copy", () => {
    state.reduced = true;
    state.slices = 12;
    state.reducedPermille = 250;
    expect(label(NaN)).toBe("Preparing a 720p copy on the server... 25%");
  });

  // "Part 4 of 12" is the whole reason the film is sliced: it turns an hour of
  // opaque waiting into visible, countable progress.
  it("counts the parts while collecting them", () => {
    state.reduced = true;
    state.slices = 12;
    state.collected = 3;
    expect(label(0.225, { downloaded: 50 * 1024 ** 2, total: 190 * 1024 ** 2 })).toBe(
      "Part 4 of 12 · 23% · 50MB / 190MB",
    );
  });

  it("switches to assembling once every part is in", () => {
    state.reduced = true;
    state.slices = 12;
    state.collected = 12;
    // The join owns 0.9..1.0 of the overall bar, shown as its own 0..100%.
    expect(label(0.95)).toBe("Assembling the film... 50%");
    expect(label(0.9)).toBe("Assembling the film... 0%");
    expect(label(1)).toBe("Assembling the film... 100%");
  });

  // Server-side audio preparation wins over the reduced stages: they cannot
  // both be true, and a label that reported the wrong one would be worse than
  // one that reported nothing.
  it("prefers the compat stage when both claim to be running", () => {
    state.compat = true;
    state.reduced = true;
    state.slices = 12;
    expect(label(NaN)).toContain("Preparing audio on the server");
  });
});
