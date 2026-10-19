import type { V1VideoPool } from "@evey/api";

// The admin load test's wire types and helpers, one copy for the web and the
// phone (each kept its own until 2026-09-24).

// --- LoadTestCard types --------------------------------------------------
//
// The backend side of this contract (POST/GET/DELETE /api/v1/admin/load-test)
// is not yet expressed as a proto/orval-generated client in this tree (the
// service is landing in a parallel worktree). Until it does, this talks to
// the endpoint through eveyFetch directly with hand-written types mirroring
// the documented wire shape - the same pattern ConcurrencyCard below already
// uses for /admin/server-settings. Swap these local types for the generated
// ones once the client lands; the shape is a 1:1 match so no behavior change
// is expected.

// The 3 "pure" scenarios exercise a single playback path; the 3 usage
// profiles mix mkvgo direct-play and transcode viewers in a fixed ratio to
// stand in for a real household's device mix ("how many different humans").
export type LoadTestScenario = "transcode" | "natural" | "mkvgo" | "light" | "balanced" | "heavy" | `clients:${string}`;

// Client mixes: each simulated viewer declares what one family of real players
// declares and the server picks its path (copy, remux, encode) as it would for
// it. "clients:history" plays the proportions of the last 30 days' playbacks.
export const CLIENT_PROFILES = ["tv", "tv-browser", "mobile", "web-chrome", "web-safari"] as const;
export type ClientProfile = (typeof CLIENT_PROFILES)[number];
export const CLIENTS_HISTORY: LoadTestScenario = "clients:history";
export const DEFAULT_CLIENT_WEIGHTS: Record<ClientProfile, number> = { tv: 40, "tv-browser": 0, mobile: 40, "web-chrome": 20, "web-safari": 0 };

// clientMixScenario turns per-profile weights into the scenario string the
// server parses ("clients:tv=40,mobile=40,web-chrome=20"); zero weights are
// left out. undefined when nothing is weighted.
export function clientMixScenario(weights: Record<ClientProfile, number>): LoadTestScenario | undefined {
  const parts = CLIENT_PROFILES.filter((p) => (weights[p] ?? 0) > 0).map((p) => `${p}=${Math.round(weights[p])}`);
  return parts.length > 0 ? `clients:${parts.join(",")}` : undefined;
}

// clientMixShares reads a resolved client scenario back into its profiles and
// whole-percent shares, for the report. undefined for any other scenario.
export function clientMixShares(scenario: string | undefined): { profile: string; pct: number }[] | undefined {
  const spec = scenario?.startsWith("clients:") ? scenario.slice("clients:".length) : undefined;
  if (!spec || spec === "history") return undefined;
  const parts = spec.split(",").map((p) => p.split("=")).map(([profile, w]) => ({ profile: profile ?? "", w: Number(w) || 0 }));
  const total = parts.reduce((n, p) => n + p.w, 0);
  return total > 0 ? parts.map((p) => ({ profile: p.profile, pct: Math.round((p.w / total) * 100) })) : undefined;
}

export const CLIENT_LABEL_KEYS: Record<string, string> = {
  tv: "admin.loadtest.client.tv",
  "tv-browser": "admin.loadtest.client.tvBrowser",
  mobile: "admin.loadtest.client.mobile",
  "web-chrome": "admin.loadtest.client.webChrome",
  "web-safari": "admin.loadtest.client.webSafari",
};

export type LoadTestStopReason =
  | "slo_breach"
  | "admission_full"
  | "cpu_saturated"
  | "memory_saturated"
  | "throughput_saturated"
  | "startup_too_slow"
  | "server_failing"
  | "library_limit"
  | "max_viewers"
  | "cancelled"
  | "token_expired"
  | "error";

// What the server was actually spending. Read from its own cgroup, so every
// ffmpeg it spawned is already counted.
export interface LoadTestResources {
  cpuPercentAvg?: number;
  cpuPercentPeak?: number;
  // The whole machine beside the container (/proc/stat): its busy share, and
  // the share of time its cores waited on I/O - the storage path saturating.
  machineCpuPercentAvg?: number;
  machineCpuPercentPeak?: number;
  machineIowaitPercentAvg?: number;
  machineIowaitPercentPeak?: number;
  // memUsedBytes* is the whole container (Meilisearch and Postgres included in
  // the -full image); processRssBytes* is the server process alone.
  memUsedBytesAvg?: number;
  memUsedBytesMax?: number;
  processRssBytesAvg?: number;
  processRssBytesMax?: number;
  memLimitBytes?: number;
  // diskReadBytesPerSec is the machine's block counters summed over the
  // devices the library sits on; absent (diskReadMeasured false) when none
  // could be tied to it. appReadBytesPerSec is what the packager asked its
  // sources for, counted in-process; absent when appReadMeasured is false.
  diskReadBytesPerSec?: number;
  diskReadMeasured?: boolean;
  appReadBytesPerSec?: number;
  appReadMeasured?: boolean;
  servedBytesPerSec?: number;
  samples?: number;
  // The server's own Go runtime over the tier: reachable heap, GOMEMLIMIT (0 =
  // unset) and the share of its CPU time spent collecting garbage.
  goHeapLiveBytesAvg?: number;
  goHeapLiveBytesMax?: number;
  goMemLimitBytes?: number;
  goGcCpuPercentAvg?: number;
  goGcCpuPercentPeak?: number;
}

export interface LoadTestDiskActivity {
  device?: string;
  utilPercentAvg?: number;
  utilPercentPeak?: number;
  readBytesPerSecAvg?: number;
  // One-second peaks and the mean queue depth (aqu-sz).
  readBytesPerSecPeak?: number;
  awaitMsAvg?: number;
  awaitMsPeak?: number;
  queueDepthAvg?: number;
}

// The busiest disk of a hold, judged against the server at rest: a read
// waiting past the bar, with the device busy or the machine in iowait to
// corroborate it. The server decides; the panel only words it.
export interface LoadTestDiskVerdict {
  device?: string;
  awaitMs?: number;
  awaitBaselineMs?: number;
  awaitMsPeak?: number;
  queueDepth?: number;
  utilPercent?: number;
  readBytesPerSec?: number;
  iowaitPercent?: number;
  saturated?: boolean;
}

// One hold of the climb: the tier, how it was observed, whether it held, what
// the machine did meanwhile, and the latencies the verdict was read on.
export interface LoadTestWindow {
  viewers?: number;
  phase?: string;
  measured?: boolean;
  broke?: boolean;
  reason?: LoadTestStopReason | "";
  detail?: string;
  resources?: LoadTestResources;
  segmentMsP50?: number;
  segmentMsP95?: number;
  // The p95 of the same fetches scaled to the reference segment length: the
  // figure the stall verdict is read on.
  segmentMsP95Scaled?: number;
  segmentSamples?: number;
  ttffMsP95?: number;
  diskVerdict?: LoadTestDiskVerdict;
  startedAt?: string;
  endedAt?: string;
}

// One audience size of the grid: how many windows it was judged on, how many
// passed, whether it held, and what the machine did over them.
export interface LoadTestGridTier {
  viewers?: number;
  windows?: number;
  passed?: number;
  held?: boolean;
  reason?: string;
  resources?: LoadTestResources;
}

export interface LoadTestStratum {
  stratum?: string;
  viewers?: number;
}

// Which block device a library location (a pool branch, a mount) is read
// from, resolved at the run's start - device letters rotate at reboot.
export interface LoadTestDeviceMapping {
  location?: string;
  device?: string;
  resolved?: string;
  disks?: string[];
}

export interface LoadTestReport {
  scenario?: LoadTestScenario;
  sustainedViewers?: number;
  stopReason?: LoadTestStopReason;
  stopDetail?: string;
  ttffMsP50?: number;
  ttffMsP95?: number;
  segmentMsP50?: number;
  segmentMsP95?: number;
  segmentSloMs?: number;
  samples?: number;
  sharedCacheViewers?: number;
  startedAt?: string;
  endedAt?: string;
  // skippedCandidates: titles that weren't mkvgo-eligible and were passed
  // over while picking media for the mkvgo/light/balanced/heavy scenarios.
  skippedCandidates?: number;
  // viewerMix: composition of the sustained rung by playback path, e.g.
  // {"mkvgo": 4, "transcode": 1} - keyed the same as segmentMsP95ByType.
  viewerMix?: Record<string, number>;
  segmentMsP95ByType?: Record<string, number>;
  // segmentSloMsByType: the latency budget each path was JUDGED against. The
  // paths don't share a segment length, so they can't share a budget.
  segmentSloMsByType?: Record<string, number>;
  // resources: what the box consumed while sustaining that viewer count - the
  // evidence behind a cpu_saturated / memory_saturated verdict.
  resources?: LoadTestResources;
  // perViewerCost: what ONE more viewer costs, as a slope across tiers.
  perViewerCost?: LoadTestResources;
  resourceBaseline?: LoadTestResources;
  // throughputCeilingBytesPerSec: the byte rate the server could not exceed. On
  // a media server this is the usual ceiling - how fast it can READ the films -
  // and no CPU or memory gauge can see it.
  throughputCeilingBytesPerSec?: number;
  diskReadStarved?: boolean;
  // coldCache: the run kept evicting the library from the page cache, so the
  // figures measure the STORAGE rather than the RAM. A warm and a cold run of
  // the same box are two different answers - never compare them as one.
  coldCache?: boolean;
  // storageFsType / storageOpaque: what the library sits on. An opaque mount
  // (a FUSE pool such as mergerfs, a network or VM share) hides the disks from
  // the server's kernel: blind disk counter, cache eviction that stops at the
  // mount, a pool counted as one disk. The panel says so before any figure.
  storageFsType?: string;
  storageOpaque?: boolean;
  // poolBranches: where the audience's titles sit on a pooled mount, one entry
  // per branch (disk), most viewers first. A pool is not a RAID - a file lives on
  // ONE disk - so a skewed placement is a skewed load.
  poolBranches?: { branch?: string; viewers?: number }[];
  // disks: what each physical drive did over the sustained tier, busiest first.
  // Read from the host-wide block counters, so it includes other readers too.
  disks?: LoadTestDiskActivity[];
  // breakViewers / breakDisks / breakCpuPercentPeak / breakResources: the
  // lowest audience that broke and what the machine did under it - where the
  // cause of a stop shows. 0 and empty when no tier broke. breakReason /
  // breakDetail name what broke it.
  breakViewers?: number;
  breakDisks?: LoadTestDiskActivity[];
  breakCpuPercentPeak?: number;
  breakResources?: LoadTestResources;
  breakReason?: LoadTestStopReason | "";
  breakDetail?: string;
  // diskVerdict: the busiest disk under the audience the stop is read on,
  // judged by the server against the baseline.
  diskVerdict?: LoadTestDiskVerdict;
  // windows: every hold the run made, in order, broken ones included.
  windows?: LoadTestWindow[];
  // diskDevices / diskReadMeasured: the location-to-device mapping the run
  // resolved, and whether any device could be tied to the library at all.
  diskDevices?: LoadTestDeviceMapping[];
  diskReadMeasured?: boolean;
  appReadMeasured?: boolean;
  // seed: the title draw's seed, as a string (it exceeds a JS number). Passing
  // it back replays the same draw.
  seed?: string;
  // grid: every audience size measured on full windows, smallest first. The
  // answer (sustainedViewers) is the largest passing four windows in five;
  // rangeLow / rangeHigh bound it (every window passed / half passed).
  grid?: LoadTestGridTier[];
  rangeLow?: number;
  rangeHigh?: number;
  // limiter: what gave out first, named by the server ("disk", "cpu",
  // "memory", "proxy", "server", "unknown", "none"); limiterLocation the
  // library location of a limiting disk; advice set only when one follows.
  limiter?: string;
  limiterLocation?: string;
  advice?: string;
  // strata: the retained audience by disk x bitrate quarter, and summed each way.
  strata?: LoadTestStratum[];
  strataByDisk?: LoadTestStratum[];
  strataByBand?: LoadTestStratum[];
  // diskReadCeilingBytesPerSec: how fast this box reads its media - every disk
  // the library spans, read at once. The kernel cannot report this when the
  // library sits on a network share or a 9p mount, so the run measures it itself
  // rather than printing a misleading zero.
  diskReadCeilingBytesPerSec?: number;
  // diskCeilingAtReaders / diskReadAtAudienceBytesPerSec: the ceiling above is
  // measured at whatever width the hardware likes (1 or 4 sequential readers on
  // spinning rust). An audience of 21 never sees it. Quoting the peak alone next
  // to the bytes actually read is how a saturated disk reads as 40% busy - so the
  // report quotes the curve at the width the audience imposes too.
  diskCeilingAtReaders?: number;
  diskReadAtAudienceBytesPerSec?: number;
  // tiers: one row per audience size the run measured - the climb itself. Every
  // cpu figure is the SERVER's share (the load generator's own overhead has been
  // subtracted), and memUsedBytes is anonymous memory, never the page cache.
  tiers?: {
    viewers?: number;
    cpuPercent?: number;
    machineCpuPercent?: number;
    machineIowaitPercent?: number;
    memUsedBytes?: number;
    processRssBytes?: number;
    diskReadBytesPerSec?: number;
    servedBytesPerSec?: number;
    samples?: number;
  }[];
  // harnessCpuFraction: how much of the measured CPU was the load GENERATOR, not
  // the server. Already subtracted from every figure above; shown so the operator
  // knows the correction happened.
  harnessCpuFraction?: number;
  harnessCpuMeasured?: boolean;
  // slowestDiskBytesPerSec: what the WEAKEST drive delivered. Several disks are
  // not one big disk - each saturates on its own, and the film a viewer picks
  // decides which.
  slowestDiskBytesPerSec?: number;
  disksProbed?: number;
  // storageCollapsesAtReaders: how many concurrent readers the storage takes
  // before it FALLS OVER (throughput halves, a single read takes seconds). On a
  // direct-play server this is roughly the number of people who can watch at
  // once before the storage - not the CPU, not Evey - makes playback crawl.
  storageCollapsesAtReaders?: number;
  // readAmplification: bytes READ from disk per byte SERVED. One is perfect;
  // four means three quarters of the storage bandwidth is spent on data the
  // server throws away - and on a direct-play box, read bandwidth IS the ceiling.
  readAmplification?: number;
  // appReadAmplification: what the SERVER asked for, per byte served. Set against
  // readAmplification (what the DISK delivered), it says whether the code is
  // greedy or the kernel is reading ahead - two problems with different fixes.
  appReadAmplification?: number;
  // servedPaths: what the server decided for each viewer type over the whole
  // run, counted once per viewer by the path its first segment came from.
  servedPaths?: { viewerType?: string; path?: string; viewers?: number }[];
  // endpoint: the origin the viewers actually hit (the reverse proxy when it was
  // reachable, the server's own loopback otherwise). Runs against different
  // endpoints are not comparable.
  endpoint?: string;
  viaProxy?: boolean;
  // proxyAddress: the local address the proxy was reached at when it sits on
  // the server's network, skipping the router's turn no outside viewer takes;
  // empty when the viewers took the public address.
  proxyAddress?: string;
  // uplink*: what the line in front of the server sent to a speed-test
  // service before the run (uplinkService), the upload outside viewers come
  // through. remoteViewers: how many of them the line and the server both
  // carry; remoteByLine: the line is the smaller.
  uplinkBytesPerSec?: number;
  uplinkMeasured?: boolean;
  uplinkService?: string;
  remoteViewers?: number;
  remoteByLine?: boolean;
}

export interface LoadTestStatus {
  running?: boolean;
  phase?: string;
  viewers?: number;
  elapsedMs?: number;
  lastReport?: LoadTestReport;
}


export const LOAD_TEST_URL = "/api/v1/admin/load-test";
export const LOAD_TEST_QUERY_KEY = ["admin-load-test-status"];

// A POST refused because real playback is active surfaces as 409 (conflict),
// 423 (locked) or 500 (the admission guard erroring out defensively) - all
// three read the same to an operator: "something is already streaming".
export const ACTIVE_STREAMS_STATUSES = [409, 423, 500];

export const STOP_REASON_KEYS: Partial<Record<LoadTestStopReason, string>> = {
  slo_breach: "admin.loadtest.stopReason.sloBreach",
  admission_full: "admin.loadtest.stopReason.admissionFull",
  cpu_saturated: "admin.loadtest.stopReason.cpuSaturated",
  memory_saturated: "admin.loadtest.stopReason.memorySaturated",
  throughput_saturated: "admin.loadtest.stopReason.throughputSaturated",
  startup_too_slow: "admin.loadtest.stopReason.startupTooSlow",
  server_failing: "admin.loadtest.stopReason.serverFailing",
  library_limit: "admin.loadtest.stopReason.libraryLimit",
  max_viewers: "admin.loadtest.stopReason.maxViewers",
  cancelled: "admin.loadtest.stopReason.cancelled",
  token_expired: "admin.loadtest.stopReason.tokenExpired",
  error: "admin.loadtest.stopReason.error",
};

// A run that ended because a RESOURCE ran out has found the machine's real
// ceiling; anything else means the ramp was stopped by something other than
// capacity (the operator's own limit, the clock, an error).
export const BOTTLENECK_REASONS: LoadTestStopReason[] = [
  "cpu_saturated",
  "memory_saturated",
  "throughput_saturated",
  "startup_too_slow",
  "server_failing",
  "slo_breach",
  "admission_full",
];

// A stop cause: the i18n key and its values. whatKey, when set, names the
// request the server failed on and is translated into the "what" value. tier
// is the audience the cause was read under and tierState says whether that
// audience held or broke - the header and the table then speak of the same
// hold.
export interface LoadTestCause {
  key: string;
  params: Record<string, string | number>;
  whatKey?: string;
  tier: number;
  tierState: "broke" | "held";
}

// The request a viewer was making when the server failed, read off the
// harness's own detail string (internal/app/loadtest/viewer.go, direct.go).
const FAILED_REQUEST_KEYS: [string, string][] = [
  ["master playlist", "admin.loadtest.cause.request.master"],
  ["variant playlist", "admin.loadtest.cause.request.variant"],
  ["segment", "admin.loadtest.cause.request.segment"],
  ["raw file", "admin.loadtest.cause.request.rawFile"],
  ["stream-info", "admin.loadtest.cause.request.streamInfo"],
];

// A CPU peak this high is the ceiling even when the stop reason was latency.
const SATURATED_CPU_PERCENT = 85;

// stopCause says WHY a run stopped, from what it measured: the server's own
// error when it failed a request outright, else the disk the server judged
// saturated (its wait per read against the baseline, not a busy percentage:
// the tiers that stalled on 2026-10-15 had their disks 56-82 % busy and every
// read waiting 27-46 ms), else a CPU at its ceiling, else that nothing was
// (the delay came from outside the box). null when the stop reason already
// names it (CPU, memory, the operator's limit, a cancel, a refusal by design).
//
// Everything is read under the audience that BROKE when one did: the
// sustained tier is by definition the one that held, so its figures look idle
// (measured 2026-10-12: disks at 15 % on the 37 that held, the 38 that broke
// unseen). The cause names that tier, so the header and the table agree.
// stopReasonKey is the stop sentence. A throughput plateau the server put on
// the proxy (nothing on the server saturated, the run went through the proxy)
// is said to be the proxy path's: "the server cannot read faster" was printed
// on 2026-10-17 beside a headline naming the proxy, disks 13 % busy.
export function stopReasonKey(report: LoadTestReport): string {
  if (report.stopReason === "throughput_saturated" && report.limiter === "proxy") {
    return "admin.loadtest.stopReason.throughputViaProxy";
  }
  return (report.stopReason && STOP_REASON_KEYS[report.stopReason]) ?? "admin.loadtest.stopReason.error";
}

export function stopCause(report: LoadTestReport): LoadTestCause | null {
  const reason = report.stopReason;
  if (!reason || !["slo_breach", "startup_too_slow", "server_failing", "throughput_saturated"].includes(reason)) {
    return null;
  }
  const atBreak = (report.breakViewers ?? 0) > 0;
  const tier = atBreak ? (report.breakViewers ?? 0) : (report.sustainedViewers ?? 0);
  const tierState = atBreak ? "broke" : "held";
  const detail = report.stopDetail ?? "";
  const status = reason === "server_failing" ? /status=(\d+)/.exec(detail)?.[1] : undefined;
  if (status && status !== "0") {
    const whatKey = FAILED_REQUEST_KEYS.find(([p]) => detail.includes(p))?.[1] ?? "admin.loadtest.cause.request.other";
    return { key: status === "410" ? "admin.loadtest.cause.http410" : "admin.loadtest.cause.httpStatus", params: { status }, whatKey, tier, tierState };
  }
  const verdict = report.diskVerdict;
  const iowait = Math.round(verdict?.iowaitPercent ?? (atBreak ? report.breakResources : report.resources)?.machineIowaitPercentAvg ?? 0);
  if (verdict?.saturated) {
    const params = {
      device: verdict.device ?? "",
      wait: Math.round(verdict.awaitMs ?? 0),
      baseline: Math.round(verdict.awaitBaselineMs ?? 0),
      queue: (verdict.queueDepth ?? 0).toFixed(1),
      util: Math.round(verdict.utilPercent ?? 0),
      iowait,
    };
    return { key: "admin.loadtest.cause.disk", params, tier, tierState };
  }
  const cpu = Math.round((atBreak ? (report.breakResources?.cpuPercentPeak ?? report.breakCpuPercentPeak) : report.resources?.cpuPercentPeak) ?? 0);
  if (cpu >= SATURATED_CPU_PERCENT) {
    return { key: "admin.loadtest.cause.cpu", params: { peak: cpu }, tier, tierState };
  }
  // Nothing on the server saturated and the server put it on the proxy: the
  // headline and the stop sentence already say so, a "the delay came from
  // elsewhere" list would be a third answer.
  if (report.limiter === "proxy") {
    return null;
  }
  if (!verdict?.device) {
    return { key: "admin.loadtest.cause.nothingSaturatedNoDisks", params: { cpu, iowait }, tier, tierState };
  }
  return {
    key: "admin.loadtest.cause.nothingSaturated",
    params: { cpu, device: verdict.device, wait: Math.round(verdict.awaitMs ?? 0), util: Math.round(verdict.utilPercent ?? 0), iowait },
    tier,
    tierState,
  };
}

// windowStateKey labels a hold's outcome for the table: held, or broke on
// which signal.
export function windowStateKey(w: LoadTestWindow): string {
  if (!w.broke) return "admin.loadtest.window.held";
  return (w.reason && STOP_REASON_SHORT_KEYS[w.reason as LoadTestStopReason]) ?? "admin.loadtest.window.broke";
}

// Short names of the break signals, for a table cell.
export const STOP_REASON_SHORT_KEYS: Partial<Record<LoadTestStopReason, string>> = {
  slo_breach: "admin.loadtest.window.brokeSlo",
  startup_too_slow: "admin.loadtest.window.brokeStartup",
  admission_full: "admin.loadtest.window.brokeAdmission",
  cpu_saturated: "admin.loadtest.window.brokeCpu",
  memory_saturated: "admin.loadtest.window.brokeMemory",
  throughput_saturated: "admin.loadtest.window.brokeThroughput",
  server_failing: "admin.loadtest.window.brokeServer",
};

// phaseKey names the climb phase a hold served.
export function phaseKey(phase: string | undefined): string {
  switch (phase) {
    // Before the first viewer: the titles are checked and the disks measured,
    // which takes about a minute on a large library. "Fast climb" there told
    // the operator a climb was under way while nobody was playing yet.
    case "starting":
      return "admin.loadtest.phase.starting";
    case "narrowing":
      return "admin.loadtest.phase.narrowing";
    case "confirming":
      return "admin.loadtest.phase.confirming";
    case "measuring":
      return "admin.loadtest.phase.measuring";
    default:
      return "admin.loadtest.phase.ramping";
  }
}

// liveElapsedMs is the run's elapsed time now: the server's figure as of the
// last poll plus the time since that answer, so the counter moves every second
// instead of jumping by the poll interval.
export function liveElapsedMs(elapsedMs: number | string | undefined, polledAt: number, now: number): number {
  const base = Number(elapsedMs ?? 0);
  if (!polledAt || now <= polledAt) return base;
  return base + (now - polledAt);
}

// deviceLines renders the location-to-device mapping, one line per location;
// an unresolved location says so rather than printing an empty device.
export function deviceLines(
  t: (key: string, opts?: Record<string, unknown>) => string,
  devices: LoadTestDeviceMapping[] | undefined,
): string[] {
  return (devices ?? []).map((m) => {
    if (!m.device) return t("admin.loadtest.deviceUnresolvedLine", { location: m.location ?? "" });
    // A volume names the physical disks it was judged on.
    const disks = (m.disks ?? []).filter((d) => d !== m.device);
    const device = disks.length > 0 ? `${m.device} (${disks.join(", ")})` : m.device;
    return t("admin.loadtest.deviceLine", { location: m.location ?? "", device });
  });
}

// Throughput is quoted in bits per second, the way a network link is.
export function formatMbps(bytesPerSec: number): string {
  return `${((bytesPerSec * 8) / 1e6).toFixed(1)} Mb/s`;
}

// poolShares turns the per-branch viewer counts into rows with their share of
// the audience, rounded to a whole percent. The share is what makes a skew
// readable: "41 viewers" means little until it is "82% of them".
export function poolShares(branches: { branch?: string; viewers?: number }[]): { branch: string; viewers: number; pct: number }[] {
  const total = branches.reduce((n, b) => n + (b.viewers ?? 0), 0);
  return branches.map((b) => ({
    branch: b.branch ?? "",
    viewers: b.viewers ?? 0,
    pct: total > 0 ? Math.round(((b.viewers ?? 0) / total) * 100) : 0,
  }));
}

// scenarioNoun is the report headline's noun for a scenario ("...in
// {{scenario}}."): a client mix names its proportions.
export function scenarioNoun(t: (key: string, opts?: Record<string, unknown>) => string, scenario: LoadTestScenario | undefined): string {
  const shares = clientMixShares(scenario);
  if (shares) {
    const mix = shares.map((s) => t("admin.loadtest.clientShare", { pct: s.pct, label: clientLabel(t, s.profile) })).join(", ");
    return t("admin.loadtest.scenarioClientsNoun", { mix });
  }
  const key = SCENARIO_NOUN_KEYS[scenario as keyof typeof SCENARIO_NOUN_KEYS] ?? SCENARIO_NOUN_KEYS.balanced;
  return t(key);
}

function clientLabel(t: (key: string, opts?: Record<string, unknown>) => string, profile: string): string {
  const key = CLIENT_LABEL_KEYS[profile];
  return key ? t(key) : profile;
}

// Path labels for servedPaths: what the server did with a viewer.
const PATH_LABEL_KEYS: Record<string, string> = {
  mkvgo: "admin.loadtest.path.mkvgo",
  "mkvgo-concat": "admin.loadtest.path.mkvgo",
  "ffmpeg-copy": "admin.loadtest.path.ffmpegCopy",
  ffmpeg: "admin.loadtest.path.ffmpegCopy",
  transcode: "admin.loadtest.path.transcode",
  direct: "admin.loadtest.path.direct",
};

// servedPathLines renders one line per viewer type: "TV: 9 copied by mkvgo,
// 2 transcoded". Types in the order the server sent them (sorted).
export function servedPathLines(
  t: (key: string, opts?: Record<string, unknown>) => string,
  paths: LoadTestReport["servedPaths"],
): string[] {
  const byType = new Map<string, string[]>();
  for (const p of paths ?? []) {
    if (!p.viewerType || !p.path || !(p.viewers ?? 0)) continue;
    const key = PATH_LABEL_KEYS[p.path];
    const part = t("admin.loadtest.mixPart", { count: p.viewers, label: key ? t(key) : p.path });
    byType.set(p.viewerType, [...(byType.get(p.viewerType) ?? []), part]);
  }
  return [...byType].map(([vt, parts]) => t("admin.loadtest.servedPathLine", { label: mixTypeLabel(t, vt), parts: parts.join(", ") }));
}

// Noun form of each fixed scenario for the report headline.
export const SCENARIO_NOUN_KEYS: Record<"transcode" | "natural" | "mkvgo" | "light" | "balanced" | "heavy", string> = {
  transcode: "admin.loadtest.scenarioTranscodeNoun",
  natural: "admin.loadtest.scenarioNaturalNoun",
  mkvgo: "admin.loadtest.scenarioMkvgoNoun",
  light: "admin.loadtest.scenarioLightNoun",
  balanced: "admin.loadtest.scenarioBalancedNoun",
  heavy: "admin.loadtest.scenarioHeavyNoun",
};

// Playback-path label for a viewerMix / segmentMsP95ByType key. Falls back to
// the raw key for any type the server reports that the panel doesn't know
// about yet, so a new backend type never renders blank.
const MIX_TYPE_LABEL_KEYS: Record<string, string> = {
  mkvgo: "admin.loadtest.mixType.mkvgo",
  transcode: "admin.loadtest.mixType.transcode",
  natural: "admin.loadtest.mixType.natural",
};

function mixTypeLabel(t: (key: string, opts?: Record<string, unknown>) => string, type: string): string {
  if (type.startsWith("client:")) return clientLabel(t, type.slice("client:".length));
  const key = MIX_TYPE_LABEL_KEYS[type];
  return key ? t(key) : type;
}

// Renders "{{count}} {{label}}" (or "{{label}} {{ms}} ms") per playback path
// present in the record, joined into one discreet line - never a table.
export function mixParts(
  t: (key: string, opts?: Record<string, unknown>) => string,
  record: Record<string, number> | undefined,
  partKey: string,
  valueField: "count" | "ms",
): string {
  return Object.entries(record ?? {})
    .filter(([, v]) => (v ?? 0) > 0)
    .map(([type, v]) => t(partKey, { [valueField]: v, label: mixTypeLabel(t, type) }))
    .join(", ");
}

export function formatElapsed(ms: number): string {
  const totalSec = Math.max(0, Math.round(ms / 1000));
  const m = Math.floor(totalSec / 60);
  const s = totalSec % 60;
  return `${m}:${String(s).padStart(2, "0")}`;
}

// predictedStreams reads the SAME calibration table VideoSessionsCard renders
// above (pools[].classes[].rungFits[]) and returns the median rung's
// max_streams for the ladder class the load test actually exercises: "hw"
// when a GPU pool exists, "software" otherwise - independent of which
// scenario the operator picked, since the pool a stream lands on is a
// hardware fact, not a test setting.
export function predictedStreams(pools: V1VideoPool[]): number | undefined {
  const gpuPool = pools.find((p) => p.pool === "gpu");
  const pool = gpuPool ?? pools.find((p) => p.pool === "cpu");
  const classes = pool?.classes ?? [];
  const wanted = gpuPool ? "hw" : "software";
  const cls = classes.find((c) => c.ladderClass === wanted) ?? classes[0];
  const rungs = cls?.rungFits ?? [];
  if (rungs.length === 0) return undefined;
  return rungs[Math.floor(rungs.length / 2)]?.maxStreams;
}

// --- The main view: words an admin who is not technical understands ---------

// What gives out first, worded for the report's main view.
const LIMITER_KEYS: Record<string, string> = {
  disk: "admin.loadtest.limiter.disk",
  storage_unmeasured: "admin.loadtest.limiter.storageUnmeasured",
  cpu: "admin.loadtest.limiter.cpu",
  memory: "admin.loadtest.limiter.memory",
  proxy: "admin.loadtest.limiter.proxy",
  server: "admin.loadtest.limiter.server",
  unknown: "admin.loadtest.limiter.unknown",
  none: "admin.loadtest.limiter.none",
};

// limiterLine is the "what gives out first" sentence, or undefined when the
// run did not get far enough to say (cancelled, an error).
export function limiterLine(t: (key: string, opts?: Record<string, unknown>) => string, report: LoadTestReport): string | undefined {
  // A proxy reached at its local address has neither the router nor the
  // upload on its path: what gave out is the proxy, or the link to it.
  const key = report.limiter === "proxy" && report.proxyAddress ? "admin.loadtest.limiter.proxyLocal" : report.limiter ? LIMITER_KEYS[report.limiter] : undefined;
  if (!key) return undefined;
  const where = report.limiter === "disk" && report.limiterLocation ? t("admin.loadtest.limiter.diskWhere", { location: report.limiterLocation }) : "";
  return t(key, { where });
}

// remoteLines say how the viewers reached the proxy and what the line
// carries to viewers outside, or nothing for a report from before either was
// measured.
export function remoteLines(t: (key: string, opts?: Record<string, unknown>) => string, report: LoadTestReport): string[] {
  const lines: string[] = [];
  if (report.proxyAddress) {
    lines.push(t("admin.loadtest.remote.viaLocal", { address: report.proxyAddress }));
  }
  if (!report.uplinkService) return lines;
  const service = report.uplinkService;
  if (!report.uplinkMeasured) {
    lines.push(t("admin.loadtest.remote.unmeasured", { service }));
    return lines;
  }
  const up = formatMbps(report.uplinkBytesPerSec ?? 0);
  const count = report.remoteViewers ?? 0;
  if (count <= 0) {
    lines.push(t("admin.loadtest.remote.lineOnly", { up, service }));
  } else {
    lines.push(t(report.remoteByLine ? "admin.loadtest.remote.byLine" : "admin.loadtest.remote.byServer", { count, up, service }));
  }
  return lines;
}

const ADVICE_KEYS: Record<string, string> = {
  spread_titles: "admin.loadtest.advice.spreadTitles",
};

// adviceLine is the one actionable advice, only when the server found one.
export function adviceLine(t: (key: string, opts?: Record<string, unknown>) => string, report: LoadTestReport): string | undefined {
  const key = report.advice ? ADVICE_KEYS[report.advice] : undefined;
  return key ? t(key) : undefined;
}

// gridRows is the main table's rows, smallest audience first: the server
// reports the bisection's candidates in the order judged.
export function gridRows(report: LoadTestReport): LoadTestGridTier[] {
  return [...(report.grid ?? [])].sort((a, b) => (a.viewers ?? 0) - (b.viewers ?? 0));
}

// verdictLine is the headline: "about N viewers in <scenario> (between A and
// B)", the range said only when it is one.
// A run that ended on an error, a cancel, an expired session or no usable
// title states a capacity only when a full window proved it: a run cut short
// after 91 held ran out its twenty minutes, and its headline must still say
// 91 (prod 2026-10-19). One that proved nothing has its stop sentence as the
// headline, never "about 0 viewers" (prod 2026-10-18).
export function verdictLine(t: (key: string, opts?: Record<string, unknown>) => string, report: LoadTestReport): string {
  if (report.stopReason && ["error", "cancelled", "token_expired", "library_limit"].includes(report.stopReason)) {
    const count = report.sustainedViewers ?? 0;
    if (count <= 0 || !(report.grid ?? []).some((g) => g.held && g.viewers === count)) {
      return t(stopReasonKey(report));
    }
    return t("admin.loadtest.verdict", { count, scenario: scenarioNoun(t, report.scenario) });
  }
  const count = report.sustainedViewers ?? 0;
  const scenario = scenarioNoun(t, report.scenario);
  const low = report.rangeLow ?? 0;
  const high = report.rangeHigh ?? 0;
  if ((report.grid?.length ?? 0) > 0 && high > 0 && (low !== count || high !== count)) {
    return t("admin.loadtest.verdictRange", { count, scenario, low: Math.min(low || count, count), high: Math.max(high, count) });
  }
  return t("admin.loadtest.verdict", { count, scenario });
}
