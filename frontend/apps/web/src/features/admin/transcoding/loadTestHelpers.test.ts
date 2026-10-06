import { describe, it, expect } from "vitest";
import type { V1VideoPool } from "@evey/api";
import { clientMixScenario, clientMixShares, formatElapsed, formatMbps, mixParts, predictedStreams, scenarioNoun, servedPathLines } from "./loadTestHelpers";

// Pure formatting/reading helpers behind the load-test panel.
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

  it("turns client weights into the scenario the server parses, leaving zeros out", () => {
    expect(clientMixScenario({ tv: 40, "tv-browser": 0, mobile: 40, "web-chrome": 20, "web-safari": 0 })).toBe("clients:tv=40,mobile=40,web-chrome=20");
    expect(clientMixScenario({ tv: 0, "tv-browser": 0, mobile: 0, "web-chrome": 0, "web-safari": 0 })).toBeUndefined();
  });

  it("reads a resolved client mix back as whole-percent shares, and nothing else", () => {
    expect(clientMixShares("clients:mobile=30,tv=12,web-chrome=8")).toEqual([
      { profile: "mobile", pct: 60 },
      { profile: "tv", pct: 24 },
      { profile: "web-chrome", pct: 16 },
    ]);
    expect(clientMixShares("clients:history")).toBeUndefined();
    expect(clientMixShares("balanced")).toBeUndefined();
  });

  it("names a client mix by its shares in the headline, and a fixed scenario by its noun", () => {
    const t = (key: string, opts?: Record<string, unknown>) => (opts ? `${key}(${Object.values(opts).join("|")})` : key);
    expect(scenarioNoun(t, "clients:tv=3,mobile=1")).toBe(
      "admin.loadtest.scenarioClientsNoun(admin.loadtest.clientShare(75|admin.loadtest.client.tv), admin.loadtest.clientShare(25|admin.loadtest.client.mobile))",
    );
    expect(scenarioNoun(t, "heavy")).toBe("admin.loadtest.scenarioHeavyNoun");
  });

  it("renders one line per viewer type with the paths the server chose", () => {
    const t = (key: string, opts?: Record<string, unknown>) => (opts ? `${key}(${Object.values(opts).join("|")})` : key);
    const lines = servedPathLines(t, [
      { viewerType: "client:tv", path: "mkvgo", viewers: 9 },
      { viewerType: "client:web-chrome", path: "mkvgo", viewers: 5 },
      { viewerType: "client:web-chrome", path: "transcode", viewers: 2 },
      { viewerType: "client:web-chrome", path: "ffmpeg-copy", viewers: 0 },
    ]);
    expect(lines).toEqual([
      "admin.loadtest.servedPathLine(admin.loadtest.client.tv|admin.loadtest.mixPart(9|admin.loadtest.path.mkvgo))",
      "admin.loadtest.servedPathLine(admin.loadtest.client.webChrome|admin.loadtest.mixPart(5|admin.loadtest.path.mkvgo), admin.loadtest.mixPart(2|admin.loadtest.path.transcode))",
    ]);
  });
});
