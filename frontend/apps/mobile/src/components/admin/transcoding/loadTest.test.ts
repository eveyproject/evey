import { describe, it, expect } from "vitest";
import type { V1VideoPool } from "@evey/api";
import { formatElapsed, formatMbps, mixParts, predictedStreams } from "./loadTest";

// Pure formatting/reading helpers behind the load-test section (mirror of the web helpers, same test).
describe("load test helpers", () => {
  it("quotes throughput in megabits, the way a link is", () => {
    expect(formatMbps(0)).toBe("0.0 Mb/s");
    expect(formatMbps(1_000_000)).toBe("8.0 Mb/s");
    expect(formatMbps(12_500_000)).toBe("100.0 Mb/s");
  });

  it("formats elapsed time as m:ss, rounded, never negative", () => {
    expect(formatElapsed(0)).toBe("0:00");
    expect(formatElapsed(59_400)).toBe("0:59");
    expect(formatElapsed(61_000)).toBe("1:01");
    expect(formatElapsed(3_599_600)).toBe("60:00");
    expect(formatElapsed(-5_000)).toBe("0:00");
  });

  it("renders one part per playback path present, skipping zeros, with the raw key for an unknown path", () => {
    const t = (key: string, opts?: Record<string, unknown>) =>
      key === "part" ? `${String(opts?.count ?? opts?.ms)} ${String(opts?.label)}` : `L(${key})`;
    const out = mixParts(t, { mkvgo: 3, transcode: 0, future: 2 }, "part", "count");
    expect(out).toBe("3 L(admin.loadtest.mixType.mkvgo), 2 future");
    expect(mixParts(t, undefined, "part", "ms")).toBe("");
  });

  it("predicts streams from the median rung of the ladder class the machine actually uses", () => {
    const cpuOnly: V1VideoPool[] = [{
      pool: "cpu",
      classes: [{ ladderClass: "software", rungFits: [{ maxStreams: 2 }, { maxStreams: 5 }, { maxStreams: 9 }] }],
    }] as V1VideoPool[];
    expect(predictedStreams(cpuOnly)).toBe(5);

    const withGpu: V1VideoPool[] = [
      ...cpuOnly,
      { pool: "gpu", classes: [{ ladderClass: "hw", rungFits: [{ maxStreams: 20 }, { maxStreams: 30 }] }] },
    ] as V1VideoPool[];
    // A GPU pool wins, and its hw class is read (median of 2 = the upper one).
    expect(predictedStreams(withGpu)).toBe(30);

    // A pool without the wanted class falls back to its first class; no rungs = unknown.
    const odd: V1VideoPool[] = [{ pool: "cpu", classes: [{ ladderClass: "other", rungFits: [{ maxStreams: 7 }] }] }] as V1VideoPool[];
    expect(predictedStreams(odd)).toBe(7);
    expect(predictedStreams([])).toBeUndefined();
    expect(predictedStreams([{ pool: "cpu", classes: [] }] as V1VideoPool[])).toBeUndefined();
  });
});
