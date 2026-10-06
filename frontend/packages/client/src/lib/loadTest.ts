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
  memUsedBytesAvg?: number;
  memUsedBytesMax?: number;
  memLimitBytes?: number;
  diskReadBytesPerSec?: number;
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
  disks?: {
    device?: string;
    utilPercentAvg?: number;
    utilPercentPeak?: number;
    readBytesPerSecAvg?: number;
    awaitMsAvg?: number;
  }[];
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
    memUsedBytes?: number;
    diskReadBytesPerSec?: number;
    servedBytesPerSec?: number;
    cpuPercentPerViewer?: number;
    memBytesPerViewer?: number;
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
