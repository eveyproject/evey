import { describe, it, expect } from "vitest";
import type { V1VideoPool } from "@evey/api";
import { clientMixScenario, clientMixShares, formatElapsed, formatMbps, limiterLine, mixParts, remoteLines, predictedStreams, scenarioNoun, liveElapsedMs, phaseKey, servedPathLines, stopCause, stopReasonKey, verdictLine } from "./loadTestHelpers";

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
      { viewerType: "client:tv", path: "direct", viewers: 7 },
      { viewerType: "client:tv", path: "mkvgo", viewers: 9 },
      { viewerType: "client:web-chrome", path: "mkvgo", viewers: 5 },
      { viewerType: "client:web-chrome", path: "transcode", viewers: 2 },
      { viewerType: "client:web-chrome", path: "ffmpeg-copy", viewers: 0 },
    ]);
    expect(lines).toEqual([
      "admin.loadtest.servedPathLine(admin.loadtest.client.tv|admin.loadtest.mixPart(7|admin.loadtest.path.direct), admin.loadtest.mixPart(9|admin.loadtest.path.mkvgo))",
      "admin.loadtest.servedPathLine(admin.loadtest.client.webChrome|admin.loadtest.mixPart(5|admin.loadtest.path.mkvgo), admin.loadtest.mixPart(2|admin.loadtest.path.transcode))",
    ]);
  });
});

// The prod reports of 2026-10-12 and the 2026-10-15 cross-check, as the
// operator saw them: a 410 on the variant playlist, a disk the server judged
// saturated on its wait per read, an idle machine whose segments still went
// slow. Every cause names the tier it was read on.
describe("stopCause", () => {
  it("names the HTTP error and the request it hit", () => {
    expect(
      stopCause({ stopReason: "server_failing", stopDetail: "variant playlist fetch failed (status=410, err=<nil>)", sustainedViewers: 37 }),
    ).toEqual({ key: "admin.loadtest.cause.http410", params: { status: "410" }, whatKey: "admin.loadtest.cause.request.variant", tier: 37, tierState: "held" });
    expect(
      stopCause({ stopReason: "server_failing", stopDetail: "master playlist fetch failed (status=502, err=<nil>)" }),
    ).toMatchObject({ key: "admin.loadtest.cause.httpStatus", params: { status: "502" }, whatKey: "admin.loadtest.cause.request.master" });
  });

  it("words the disk the server judged saturated, with its wait against the baseline", () => {
    expect(
      stopCause({
        stopReason: "slo_breach",
        sustainedViewers: 126,
        breakViewers: 131,
        breakResources: { cpuPercentPeak: 13, machineIowaitPercentAvg: 65 },
        diskVerdict: { device: "sda", awaitMs: 27.4, awaitBaselineMs: 7.2, awaitMsPeak: 46, queueDepth: 3.46, utilPercent: 68.4, iowaitPercent: 65.2, saturated: true },
      }),
    ).toEqual({
      key: "admin.loadtest.cause.disk",
      params: { device: "sda", wait: 27, baseline: 7, queue: "3.5", util: 68, iowait: 65 },
      tier: 131,
      tierState: "broke",
    });
  });

  it("does not call a busy disk saturated when the server did not", () => {
    // 2026-10-15: 56-82 % busy was under the old 80 % bar and the reads
    // waited 27-46 ms; now the server judges, and a disk busy but quick is
    // not the cause.
    expect(
      stopCause({
        stopReason: "slo_breach",
        sustainedViewers: 78,
        resources: { cpuPercentPeak: 4, machineIowaitPercentAvg: 20.6 },
        diskVerdict: { device: "sda", awaitMs: 8.4, awaitBaselineMs: 6, utilPercent: 85, iowaitPercent: 20.6, saturated: false },
      }),
    ).toEqual({
      key: "admin.loadtest.cause.nothingSaturated",
      params: { cpu: 4, device: "sda", wait: 8, util: 85, iowait: 21 },
      tier: 78,
      tierState: "held",
    });
    expect(stopCause({ stopReason: "slo_breach", sustainedViewers: 12, resources: { cpuPercentPeak: 91 } })).toEqual({
      key: "admin.loadtest.cause.cpu",
      params: { peak: 91 },
      tier: 12,
      tierState: "held",
    });
    expect(stopCause({ stopReason: "slo_breach", sustainedViewers: 12, resources: { cpuPercentPeak: 3, machineIowaitPercentAvg: 1.4 } })).toEqual({
      key: "admin.loadtest.cause.nothingSaturatedNoDisks",
      params: { cpu: 3, iowait: 1 },
      tier: 12,
      tierState: "held",
    });
  });

  it("reads the CPU of the tier that broke, not the one that held", () => {
    expect(
      stopCause({
        stopReason: "startup_too_slow",
        sustainedViewers: 37,
        resources: { cpuPercentPeak: 4 },
        breakViewers: 38,
        breakCpuPercentPeak: 92,
      }),
    ).toEqual({ key: "admin.loadtest.cause.cpu", params: { peak: 92 }, tier: 38, tierState: "broke" });
  });

  it("adds nothing when the stop reason already says it", () => {
    for (const stopReason of ["cpu_saturated", "max_viewers", "cancelled", "admission_full"] as const) {
      expect(stopCause({ stopReason })).toBeNull();
    }
  });
});

describe("stopReasonKey", () => {
  // The 2026-10-17 run through the proxy: 49 held, 51 broke on a throughput
  // plateau near 255 Mb/s, CPU 9 %, sda 7 ms and 13 % busy, the server naming
  // the proxy - and the report saying the server could not read any faster.
  const viaProxy = {
    stopReason: "throughput_saturated" as const,
    limiter: "proxy",
    viaProxy: true,
    sustainedViewers: 49,
    breakViewers: 51,
    breakResources: { cpuPercentPeak: 9, machineIowaitPercentAvg: 14 },
    diskVerdict: { device: "sda", awaitMs: 7, utilPercent: 13, iowaitPercent: 14, saturated: false },
  };

  it("puts a plateau the server named on the proxy on the proxy path, with no third cause", () => {
    expect(stopReasonKey(viaProxy)).toBe("admin.loadtest.stopReason.throughputViaProxy");
    expect(stopCause(viaProxy)).toBeNull();
  });

  it("keeps the server's own plateau and its cause when the run went direct", () => {
    const direct = { ...viaProxy, limiter: "unknown", viaProxy: false };
    expect(stopReasonKey(direct)).toBe("admin.loadtest.stopReason.throughputSaturated");
    expect(stopCause(direct)).toMatchObject({ key: "admin.loadtest.cause.nothingSaturated", tier: 51, tierState: "broke" });
  });

  it("falls back to the generic error sentence without a stop reason", () => {
    expect(stopReasonKey({})).toBe("admin.loadtest.stopReason.error");
  });
});

describe("verdictLine", () => {
  const t = (key: string, opts?: Record<string, unknown>) => (opts && "count" in opts ? `${key}:${opts.count}` : key);

  // Prod 2026-10-18: the first viewer never got an answer, and the headline
  // still said "about 0 viewers". A run that measured nothing states no count.
  it("headlines the stop sentence, not a count, when the run ended on an error", () => {
    const report = { stopReason: "error" as const, sustainedViewers: 0, rangeLow: 0, rangeHigh: 1, grid: [{ viewers: 1, held: false }] };
    expect(verdictLine(t, report)).toBe("admin.loadtest.stopReason.error");
  });

  // Prod 2026-10-19: 91 held a full window, then the run ran out its twenty
  // minutes; the headline said "Cancelled." over a table proving 91.
  it("keeps a count a full window proved when the run was cut short", () => {
    for (const stopReason of ["cancelled", "library_limit"] as const) {
      const report = { stopReason, sustainedViewers: 91, grid: [{ viewers: 81, held: true }, { viewers: 91, held: true }, { viewers: 101, held: false }] };
      expect(verdictLine(t, report)).toBe("admin.loadtest.verdict:91");
    }
  });

  it("states no count for a run cut short before any window held it", () => {
    const report = { stopReason: "cancelled" as const, sustainedViewers: 30, grid: [{ viewers: 21, held: true }] };
    expect(verdictLine(t, report)).toBe("admin.loadtest.stopReason.cancelled");
  });

  it("keeps the count for a run that measured a capacity", () => {
    expect(verdictLine(t, { stopReason: "slo_breach" as const, sustainedViewers: 12 })).toMatch(/^admin\.loadtest\.verdict.*:12$/);
  });
});

describe("running status", () => {
  // The status line printed the server's raw word ("phase: narrowing"), and
  // the minute of checks before the first viewer read as a fast climb.
  it("names every phase with a translation key, the preparation included", () => {
    expect(phaseKey("starting")).toBe("admin.loadtest.phase.starting");
    expect(phaseKey("narrowing")).toBe("admin.loadtest.phase.narrowing");
    expect(phaseKey("ramping")).toBe("admin.loadtest.phase.ramping");
  });

  // The counter jumped by the poll interval; it now adds the time since the
  // last answer.
  it("moves the elapsed time between polls", () => {
    expect(liveElapsedMs("57000", 1_000_000, 1_001_500)).toBe(58_500);
    expect(liveElapsedMs(57_000, 1_000_000, 1_000_000)).toBe(57_000);
    expect(liveElapsedMs(undefined, 0, 5_000)).toBe(0);
  });
});

describe("remoteLines", () => {
  const t = (key: string, opts?: Record<string, unknown>) => (opts ? `${key}(${Object.values(opts).join("|")})` : key);

  it("says nothing for a report from before the line was measured", () => {
    expect(remoteLines(t, { sustainedViewers: 93 })).toEqual([]);
  });

  it("names the proxy's local address, then the line and who limits", () => {
    expect(
      remoteLines(t, { proxyAddress: "192.168.1.201:443", uplinkService: "speed.cloudflare.com", uplinkMeasured: true, uplinkBytesPerSec: 37_500_000, remoteViewers: 44, remoteByLine: true }),
    ).toEqual(["admin.loadtest.remote.viaLocal(192.168.1.201:443)", "admin.loadtest.remote.byLine(44|300.0 Mb/s|speed.cloudflare.com)"]);
    expect(remoteLines(t, { uplinkService: "speed.cloudflare.com", uplinkMeasured: true, uplinkBytesPerSec: 87_500_000, remoteViewers: 93 })).toEqual([
      "admin.loadtest.remote.byServer(93|700.0 Mb/s|speed.cloudflare.com)",
    ]);
  });

  it("says the line could not be measured rather than printing a zero", () => {
    expect(remoteLines(t, { uplinkService: "speed.cloudflare.com", uplinkMeasured: false })).toEqual(["admin.loadtest.remote.unmeasured(speed.cloudflare.com)"]);
  });

  it("gives the line alone when no audience held", () => {
    expect(remoteLines(t, { uplinkService: "speed.cloudflare.com", uplinkMeasured: true, uplinkBytesPerSec: 87_500_000, remoteViewers: 0 })).toEqual([
      "admin.loadtest.remote.lineOnly(700.0 Mb/s|speed.cloudflare.com)",
    ]);
  });
});

describe("limiterLine for a proxy", () => {
  const t = (key: string) => key;
  it("blames the proxy itself when it was reached at its local address, the public path otherwise", () => {
    expect(limiterLine(t, { limiter: "proxy", proxyAddress: "192.168.1.201:443" })).toBe("admin.loadtest.limiter.proxyLocal");
    expect(limiterLine(t, { limiter: "proxy" })).toBe("admin.loadtest.limiter.proxy");
  });
});
